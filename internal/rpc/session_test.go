package rpc_test

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"

	chatv1 "github.com/reno/go-termchat/gen/go/chat/v1"
	"github.com/reno/go-termchat/gen/go/chat/v1/chatv1connect"
	"github.com/reno/go-termchat/internal/chat"
	"github.com/reno/go-termchat/internal/config"
	"github.com/reno/go-termchat/internal/hub"
	"github.com/reno/go-termchat/internal/rpc"
	"github.com/reno/go-termchat/internal/session"
	"github.com/reno/go-termchat/internal/store"
)

type testClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *testClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *testClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

type harness struct {
	server    *httptest.Server
	store     *store.Memory
	hub       *hub.Hub
	manager   *session.Manager
	clock     *testClock
	ttl       time.Duration
	ids       *chat.IDGen
	capacity  int
	burst     int
	refill    time.Duration
	typingTTL time.Duration
}

func newHarness(t *testing.T) *harness {
	t.Helper()

	const (
		ttl       = 30 * time.Minute
		capacity  = 8
		burst     = 5
		refill    = 2 * time.Second
		typingTTL = 5 * time.Second
	)

	memory := store.NewMemory(config.Retention{
		MessagesPerChannel: capacity,
		MessageTTL:         time.Hour,
		SweepInterval:      time.Minute,
	})

	signer, err := session.NewSigner(bytes.Repeat([]byte{0x2a}, session.MinKeyLen))
	if err != nil {
		t.Fatalf("NewSigner: %v", err)
	}

	clock := &testClock{now: time.Now()}
	manager := session.NewManager(
		memory, signer, chat.NewRules(chat.DefaultLimits()),
		session.NewCookies(""), ttl,
		session.WithClock(clock),
	)

	ids := chat.NewIDGen()
	events := hub.New(16)
	limiter := chat.NewRateLimiter(chat.Rate{
		Burst:  burst,
		Refill: refill,
		Idle:   5 * time.Minute,
	})

	server := httptest.NewTLSServer(rpc.NewMux(rpc.Services{
		Sessions: rpc.NewSessionHandler(manager),
		Chat: rpc.NewChatHandler(
			memory, events, chat.NewRules(chat.DefaultLimits()),
			limiter, ids, typingTTL, rpc.WithChatClock(clock),
		),
		Manager: manager,
	}))
	t.Cleanup(server.Close)

	return &harness{
		server:    server,
		store:     memory,
		hub:       events,
		manager:   manager,
		clock:     clock,
		ttl:       ttl,
		ids:       ids,
		capacity:  capacity,
		burst:     burst,
		refill:    refill,
		typingTTL: typingTTL,
	}
}

func (h *harness) chatClient(t *testing.T, nick string) chatv1connect.ChatServiceClient {
	t.Helper()

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookiejar: %v", err)
	}
	httpClient := h.httpClient(t, jar)

	sessions := chatv1connect.NewSessionServiceClient(httpClient, h.server.URL)
	if _, err := sessions.Join(t.Context(), connect.NewRequest(&chatv1.JoinRequest{Nick: nick})); err != nil {
		t.Fatalf("Join(%q): %v", nick, err)
	}
	return chatv1connect.NewChatServiceClient(httpClient, h.server.URL)
}

func (h *harness) subscribe(t *testing.T, nick string) *hub.Subscription {
	t.Helper()

	current, err := h.store.SessionByNick(t.Context(), nick)
	if err != nil {
		joined, _, joinErr := h.manager.Join(t.Context(), nick)
		if joinErr != nil {
			t.Fatalf("Join(%q): %v", nick, joinErr)
		}
		current = joined
	}

	sub := h.hub.Subscribe(current.ID)
	t.Cleanup(func() { h.hub.Unsubscribe(sub) })
	return sub
}

func (h *harness) subscribeToChannel(t *testing.T, nick, channelID string) *hub.Subscription {
	t.Helper()

	sub := h.subscribe(t, nick)
	h.hub.JoinChannel(sub.SessionID(), channelID)
	return sub
}

func (h *harness) newID(t *testing.T) string {
	t.Helper()

	id, err := h.ids.New(h.clock.Now())
	if err != nil {
		t.Fatalf("new id: %v", err)
	}
	return id
}

func (h *harness) joinChannel(t *testing.T, client chatv1connect.ChatServiceClient, name string) string {
	t.Helper()

	res, err := client.JoinChannel(t.Context(), connect.NewRequest(&chatv1.JoinChannelRequest{Name: name}))
	if err != nil {
		t.Fatalf("JoinChannel(%q): %v", name, err)
	}
	return res.Msg.GetChannel().GetId()
}

func (h *harness) send(t *testing.T, client chatv1connect.ChatServiceClient, channelID, body string) *chatv1.Message {
	t.Helper()

	h.clock.Advance(h.refill)

	res, err := client.SendMessage(t.Context(), connect.NewRequest(&chatv1.SendMessageRequest{
		ChannelId:       channelID,
		ClientMessageId: h.newID(t),
		Body:            body,
		Kind:            chatv1.MessageKind_MESSAGE_KIND_NORMAL,
	}))
	if err != nil {
		t.Fatalf("SendMessage(%q): %v", body, err)
	}
	return res.Msg.GetMessage()
}

func (h *harness) client(t *testing.T) chatv1connect.SessionServiceClient {
	t.Helper()

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookiejar: %v", err)
	}
	return chatv1connect.NewSessionServiceClient(h.httpClient(t, jar), h.server.URL)
}

func (h *harness) httpClient(t *testing.T, jar http.CookieJar) *http.Client {
	t.Helper()

	transport, ok := h.server.Client().Transport.(*http.Transport)
	if !ok {
		t.Fatal("httptest server client is not using an *http.Transport")
	}
	cloned := transport.Clone()
	t.Cleanup(cloned.CloseIdleConnections)
	return &http.Client{Transport: cloned, Jar: jar}
}

func (h *harness) post(t *testing.T, body string) (*http.Response, error) {
	t.Helper()

	req, err := http.NewRequestWithContext(
		t.Context(), http.MethodPost,
		h.server.URL+chatv1connect.SessionServiceJoinProcedure,
		bytes.NewReader([]byte(body)),
	)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	return h.httpClient(t, nil).Do(req)
}

func (h *harness) join(t *testing.T, nick string) string {
	t.Helper()

	res, err := h.post(t, `{"nick":"`+nick+`"}`)
	if err != nil {
		t.Fatalf("Join: %v", err)
	}
	t.Cleanup(func() { _ = res.Body.Close() })

	cookies := res.Cookies()
	if len(cookies) != 1 {
		t.Fatalf("Join returned %d cookies, want 1", len(cookies))
	}
	return cookies[0].Value
}

func codeOf(t *testing.T, err error) connect.Code {
	t.Helper()

	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	return connect.CodeOf(err)
}

func errorDetailOf(t *testing.T, err error) *chatv1.ErrorDetail {
	t.Helper()

	var connectErr *connect.Error
	if !errors.As(err, &connectErr) {
		t.Fatalf("not a connect error: %v", err)
	}
	for _, detail := range connectErr.Details() {
		value, valueErr := detail.Value()
		if valueErr != nil {
			continue
		}
		if errorDetail, ok := value.(*chatv1.ErrorDetail); ok {
			return errorDetail
		}
	}
	t.Fatalf("no chat.v1.ErrorDetail attached to %v", err)
	return nil
}

func detailLimit(t *testing.T, err error) uint32 {
	t.Helper()

	return errorDetailOf(t, err).GetLimit()
}

func detailRetryAfter(t *testing.T, err error) time.Duration {
	t.Helper()

	return errorDetailOf(t, err).GetRetryAfter().AsDuration()
}

func detailReason(t *testing.T, err error) chatv1.ErrorReason {
	t.Helper()

	var connectErr *connect.Error
	if !errors.As(err, &connectErr) {
		t.Fatalf("not a connect error: %v", err)
	}
	for _, detail := range connectErr.Details() {
		value, valueErr := detail.Value()
		if valueErr != nil {
			continue
		}
		if errorDetail, ok := value.(*chatv1.ErrorDetail); ok {
			return errorDetail.GetReason()
		}
	}
	t.Fatalf("no chat.v1.ErrorDetail attached to %v", err)
	return chatv1.ErrorReason_ERROR_REASON_UNSPECIFIED
}

func TestJoinThenAuthedCallThenSetNickThenLeave(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	client := h.client(t)
	ctx := t.Context()

	joined, err := client.Join(ctx, connect.NewRequest(&chatv1.JoinRequest{Nick: "rill"}))
	if err != nil {
		t.Fatalf("Join: %v", err)
	}
	if got := joined.Msg.GetSession().GetNick(); got != "rill" {
		t.Errorf("joined nick = %q", got)
	}

	got, err := client.GetSession(ctx, connect.NewRequest(&chatv1.GetSessionRequest{}))
	if err != nil {
		t.Fatalf("GetSession after Join: %v", err)
	}
	if nick := got.Msg.GetSession().GetNick(); nick != "rill" {
		t.Errorf("session nick = %q", nick)
	}

	renamed, err := client.SetNick(ctx, connect.NewRequest(&chatv1.SetNickRequest{Nick: "0x9c"}))
	if err != nil {
		t.Fatalf("SetNick: %v", err)
	}
	if nick := renamed.Msg.GetSession().GetNick(); nick != "0x9c" {
		t.Errorf("renamed nick = %q", nick)
	}

	if _, leaveErr := client.Leave(ctx, connect.NewRequest(&chatv1.LeaveRequest{})); leaveErr != nil {
		t.Fatalf("Leave: %v", leaveErr)
	}

	_, err = client.GetSession(ctx, connect.NewRequest(&chatv1.GetSessionRequest{}))
	if code := codeOf(t, err); code != connect.CodeUnauthenticated {
		t.Errorf("GetSession after Leave = %s, want unauthenticated", code)
	}
}

func TestJoinIsTheOnlyAnonymousMethod(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	client := h.client(t)
	ctx := t.Context()

	tests := []struct {
		name string
		call func() error
	}{
		{name: "GetSession", call: func() error {
			_, err := client.GetSession(ctx, connect.NewRequest(&chatv1.GetSessionRequest{}))
			return err
		}},
		{name: "SetNick", call: func() error {
			_, err := client.SetNick(ctx, connect.NewRequest(&chatv1.SetNickRequest{Nick: "rill"}))
			return err
		}},
		{name: "Leave", call: func() error {
			_, err := client.Leave(ctx, connect.NewRequest(&chatv1.LeaveRequest{}))
			return err
		}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if code := codeOf(t, tc.call()); code != connect.CodeUnauthenticated {
				t.Errorf("%s without a cookie = %s, want unauthenticated", tc.name, code)
			}
		})
	}
}

func TestJoinSetsAnHTTPOnlyCookie(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	raw, err := h.post(t, `{"nick":"rill"}`)
	if err != nil {
		t.Fatalf("Join: %v", err)
	}
	t.Cleanup(func() { _ = raw.Body.Close() })

	cookies := raw.Cookies()
	if len(cookies) != 1 {
		t.Fatalf("got %d cookies, want 1", len(cookies))
	}
	cookie := cookies[0]

	if cookie.Name != session.DefaultCookieName {
		t.Errorf("cookie name = %q", cookie.Name)
	}
	if !cookie.HttpOnly {
		t.Error("cookie is not HttpOnly")
	}
	if !cookie.Secure {
		t.Error("cookie is not Secure")
	}
	if cookie.SameSite != http.SameSiteLaxMode {
		t.Errorf("SameSite = %v, want Lax", cookie.SameSite)
	}
	if cookie.Value == "" {
		t.Error("cookie carries no token")
	}
}

func TestATamperedCookieIsUnauthenticated(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	ctx := t.Context()

	token := h.join(t, "rill")

	tests := []struct {
		name  string
		token string
	}{
		{name: "flipped last character", token: token[:len(token)-1] + "A"},
		{name: "signature stripped", token: token[:22]},
		{name: "empty", token: ""},
		{name: "unrelated value", token: "not-a-token"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := connect.NewRequest(&chatv1.GetSessionRequest{})
			req.Header().Set("Cookie", session.DefaultCookieName+"="+tc.token)

			client := chatv1connect.NewSessionServiceClient(h.httpClient(t, nil), h.server.URL)
			_, err := client.GetSession(ctx, req)
			if code := codeOf(t, err); code != connect.CodeUnauthenticated {
				t.Errorf("code = %s, want unauthenticated", code)
			}
		})
	}
}

func TestNickCollisionIsReportedWithATypedDetail(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	ctx := t.Context()

	first := h.client(t)
	if _, err := first.Join(ctx, connect.NewRequest(&chatv1.JoinRequest{Nick: "rill"})); err != nil {
		t.Fatalf("first Join: %v", err)
	}

	second := h.client(t)
	_, err := second.Join(ctx, connect.NewRequest(&chatv1.JoinRequest{Nick: "RILL"}))

	if code := codeOf(t, err); code != connect.CodeAlreadyExists {
		t.Errorf("code = %s, want already_exists", code)
	}
	if reason := detailReason(t, err); reason != chatv1.ErrorReason_ERROR_REASON_NICK_TAKEN {
		t.Errorf("reason = %s, want NICK_TAKEN", reason)
	}
}

func TestNickCollisionIsNotSilentlySuffixed(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	ctx := t.Context()

	first := h.client(t)
	if _, err := first.Join(ctx, connect.NewRequest(&chatv1.JoinRequest{Nick: "rill"})); err != nil {
		t.Fatalf("first Join: %v", err)
	}

	second := h.client(t)
	res, err := second.Join(ctx, connect.NewRequest(&chatv1.JoinRequest{Nick: "rill"}))
	if err == nil {
		t.Fatalf("second Join succeeded with nick %q, want rejection",
			res.Msg.GetSession().GetNick())
	}
}

func TestAnInvalidNickIsRejectedWithADetail(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	ctx := t.Context()

	tests := []struct {
		name string
		nick string
	}{
		{name: "empty", nick: ""},
		{name: "whitespace", nick: "   "},
		{name: "inner space", nick: "ri ll"},
		{name: "punctuation", nick: "ri!ll"},
		{name: "too long", nick: "abcdefghijklmnopqrstuvwxyz"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			client := h.client(t)
			_, err := client.Join(ctx, connect.NewRequest(&chatv1.JoinRequest{Nick: tc.nick}))

			if code := codeOf(t, err); code != connect.CodeInvalidArgument {
				t.Errorf("code = %s, want invalid_argument", code)
			}
			if reason := detailReason(t, err); reason != chatv1.ErrorReason_ERROR_REASON_NICK_INVALID {
				t.Errorf("reason = %s, want NICK_INVALID", reason)
			}
		})
	}
}

func TestLeaveReleasesTheNickForSomeoneElse(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	ctx := t.Context()

	first := h.client(t)
	if _, err := first.Join(ctx, connect.NewRequest(&chatv1.JoinRequest{Nick: "rill"})); err != nil {
		t.Fatalf("first Join: %v", err)
	}
	if _, err := first.Leave(ctx, connect.NewRequest(&chatv1.LeaveRequest{})); err != nil {
		t.Fatalf("Leave: %v", err)
	}

	second := h.client(t)
	if _, err := second.Join(ctx, connect.NewRequest(&chatv1.JoinRequest{Nick: "rill"})); err != nil {
		t.Errorf("nick not released by Leave: %v", err)
	}
}

func TestLeaveRevokesTheCookieRatherThanRefreshingIt(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	ctx := t.Context()

	token := h.join(t, "rill")

	req := connect.NewRequest(&chatv1.LeaveRequest{})
	req.Header().Set("Cookie", session.DefaultCookieName+"="+token)

	client := chatv1connect.NewSessionServiceClient(h.httpClient(t, nil), h.server.URL)
	res, err := client.Leave(ctx, req)
	if err != nil {
		t.Fatalf("Leave: %v", err)
	}

	setCookies := res.Header().Values("Set-Cookie")
	if len(setCookies) != 1 {
		t.Fatalf("got %d Set-Cookie headers, want exactly 1: %v", len(setCookies), setCookies)
	}
	revoked, err := http.ParseSetCookie(setCookies[0])
	if err != nil {
		t.Fatalf("ParseSetCookie: %v", err)
	}
	if revoked.Value != "" || revoked.MaxAge >= 0 {
		t.Errorf("Leave refreshed the cookie instead of revoking it: %+v", revoked)
	}
}

func TestSlidingTTLKeepsAnActiveSessionAlive(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	client := h.client(t)
	ctx := t.Context()

	if _, err := client.Join(ctx, connect.NewRequest(&chatv1.JoinRequest{Nick: "rill"})); err != nil {
		t.Fatalf("Join: %v", err)
	}

	for i := range 5 {
		h.clock.Advance(h.ttl - time.Minute)
		if _, err := client.GetSession(ctx, connect.NewRequest(&chatv1.GetSessionRequest{})); err != nil {
			t.Fatalf("GetSession on activity %d: %v", i, err)
		}
	}

	h.clock.Advance(h.ttl + time.Minute)
	_, err := client.GetSession(ctx, connect.NewRequest(&chatv1.GetSessionRequest{}))
	if code := codeOf(t, err); code != connect.CodeUnauthenticated {
		t.Errorf("idle past the TTL = %s, want unauthenticated", code)
	}
}

func TestAnExpiredSessionReleasesItsNick(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	ctx := t.Context()

	first := h.client(t)
	if _, err := first.Join(ctx, connect.NewRequest(&chatv1.JoinRequest{Nick: "rill"})); err != nil {
		t.Fatalf("Join: %v", err)
	}

	h.clock.Advance(h.ttl + time.Minute)

	if _, err := first.GetSession(ctx, connect.NewRequest(&chatv1.GetSessionRequest{})); err == nil {
		t.Fatal("expired session still authenticated")
	}

	second := h.client(t)
	if _, err := second.Join(ctx, connect.NewRequest(&chatv1.JoinRequest{Nick: "rill"})); err != nil {
		t.Errorf("nick not released when the session expired: %v", err)
	}
}

func TestConcurrentJoinsForOneNickProduceOneWinner(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	ctx := t.Context()

	const racers = 25

	var (
		wg    sync.WaitGroup
		mu    sync.Mutex
		wins  int
		start = make(chan struct{})
	)

	wg.Add(racers)
	for range racers {
		client := h.client(t)
		go func() {
			defer wg.Done()
			<-start
			if _, err := client.Join(ctx, connect.NewRequest(&chatv1.JoinRequest{Nick: "rill"})); err == nil {
				mu.Lock()
				wins++
				mu.Unlock()
			}
		}()
	}
	close(start)
	wg.Wait()

	if wins != 1 {
		t.Errorf("%d clients claimed the same nick, want exactly 1", wins)
	}
}
