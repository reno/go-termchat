package rpc_test

import (
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"

	chatv1 "github.com/reno/go-termchat/gen/go/chat/v1"
	"github.com/reno/go-termchat/internal/chat"
	"github.com/reno/go-termchat/internal/rpc"
)

func TestJoinChannelCreatesOnDemandAndSeedsTheDesignsSystemLine(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	client := h.chatClient(t, "rill")
	ctx := t.Context()

	joined, err := client.JoinChannel(ctx, connect.NewRequest(&chatv1.JoinChannelRequest{Name: "foo"}))
	if err != nil {
		t.Fatalf("JoinChannel: %v", err)
	}
	if !joined.Msg.GetCreated() {
		t.Error("first join did not report created")
	}
	if name := joined.Msg.GetChannel().GetName(); name != "?foo" {
		t.Errorf("channel name = %q, want ?foo", name)
	}

	page, err := client.ListMessages(ctx, connect.NewRequest(&chatv1.ListMessagesRequest{
		ChannelId: joined.Msg.GetChannel().GetId(),
	}))
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	if len(page.Msg.GetMessages()) != 1 {
		t.Fatalf("got %d messages, want the seeded system line", len(page.Msg.GetMessages()))
	}

	notice := page.Msg.GetMessages()[0]
	if want := "foo" + rpc.CreatedChannelSuffix; notice.GetBody() != want {
		t.Errorf("body = %q, want %q", notice.GetBody(), want)
	}
	if notice.GetKind() != chatv1.MessageKind_MESSAGE_KIND_SYSTEM {
		t.Errorf("kind = %s, want SYSTEM", notice.GetKind())
	}
	if notice.GetAuthorNick() != "*" {
		t.Errorf("author = %q, want *", notice.GetAuthorNick())
	}
}

func TestJoinChannelIsIdempotentAndSeedsOnce(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	first := h.chatClient(t, "rill")
	second := h.chatClient(t, "anon2")
	ctx := t.Context()

	created, err := first.JoinChannel(ctx, connect.NewRequest(&chatv1.JoinChannelRequest{Name: "?lounge"}))
	if err != nil {
		t.Fatalf("first join: %v", err)
	}

	rejoined, err := second.JoinChannel(ctx, connect.NewRequest(&chatv1.JoinChannelRequest{Name: "lounge"}))
	if err != nil {
		t.Fatalf("second join: %v", err)
	}
	if rejoined.Msg.GetCreated() {
		t.Error("second join reported created")
	}
	if rejoined.Msg.GetChannel().GetId() != created.Msg.GetChannel().GetId() {
		t.Error("the two joins produced different channels")
	}

	page, err := first.ListMessages(ctx, connect.NewRequest(&chatv1.ListMessagesRequest{
		ChannelId: created.Msg.GetChannel().GetId(),
	}))
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	if len(page.Msg.GetMessages()) != 1 {
		t.Errorf("got %d system lines, want exactly 1", len(page.Msg.GetMessages()))
	}
}

func TestChannelNameIsNormalisedBeforeCreation(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	client := h.chatClient(t, "rill")
	ctx := t.Context()

	for _, name := range []string{"Lounge", "?LOUNGE", "  ?lounge  "} {
		res, err := client.JoinChannel(ctx, connect.NewRequest(&chatv1.JoinChannelRequest{Name: name}))
		if err != nil {
			t.Fatalf("JoinChannel(%q): %v", name, err)
		}
		if got := res.Msg.GetChannel().GetName(); got != "?lounge" {
			t.Errorf("JoinChannel(%q) named it %q", name, got)
		}
	}

	channels, err := client.ListChannels(ctx, connect.NewRequest(&chatv1.ListChannelsRequest{}))
	if err != nil {
		t.Fatalf("ListChannels: %v", err)
	}
	if len(channels.Msg.GetChannels()) != 1 {
		t.Errorf("got %d channels, want 1", len(channels.Msg.GetChannels()))
	}
}

func TestListChannelsAndMembersReportCounts(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	first := h.chatClient(t, "rill")
	second := h.chatClient(t, "anon2")
	ctx := t.Context()

	joined, err := first.JoinChannel(ctx, connect.NewRequest(&chatv1.JoinChannelRequest{Name: "teste"}))
	if err != nil {
		t.Fatalf("first join: %v", err)
	}
	channelID := joined.Msg.GetChannel().GetId()

	if _, joinErr := second.JoinChannel(ctx, connect.NewRequest(&chatv1.JoinChannelRequest{Name: "teste"})); joinErr != nil {
		t.Fatalf("second join: %v", joinErr)
	}

	channels, err := first.ListChannels(ctx, connect.NewRequest(&chatv1.ListChannelsRequest{}))
	if err != nil {
		t.Fatalf("ListChannels: %v", err)
	}
	if got := channels.Msg.GetChannels()[0].GetMemberCount(); got != 2 {
		t.Errorf("member count = %d, want 2", got)
	}

	members, err := first.ListMembers(ctx, connect.NewRequest(&chatv1.ListMembersRequest{ChannelId: channelID}))
	if err != nil {
		t.Fatalf("ListMembers: %v", err)
	}
	if len(members.Msg.GetMembers()) != 2 {
		t.Fatalf("got %d members, want 2", len(members.Msg.GetMembers()))
	}
	if members.Msg.GetMembers()[0].GetPresence() != chatv1.Presence_PRESENCE_ACTIVE {
		t.Error("joined member is not active")
	}
}

func TestLeaveChannelRemovesTheMember(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	first := h.chatClient(t, "rill")
	second := h.chatClient(t, "anon2")
	ctx := t.Context()

	joined, err := first.JoinChannel(ctx, connect.NewRequest(&chatv1.JoinChannelRequest{Name: "teste"}))
	if err != nil {
		t.Fatalf("first join: %v", err)
	}
	channelID := joined.Msg.GetChannel().GetId()

	if _, joinErr := second.JoinChannel(ctx, connect.NewRequest(&chatv1.JoinChannelRequest{Name: "teste"})); joinErr != nil {
		t.Fatalf("second join: %v", joinErr)
	}
	if _, leaveErr := second.LeaveChannel(ctx, connect.NewRequest(&chatv1.LeaveChannelRequest{ChannelId: channelID})); leaveErr != nil {
		t.Fatalf("LeaveChannel: %v", leaveErr)
	}

	members, err := first.ListMembers(ctx, connect.NewRequest(&chatv1.ListMembersRequest{ChannelId: channelID}))
	if err != nil {
		t.Fatalf("ListMembers: %v", err)
	}
	if len(members.Msg.GetMembers()) != 1 {
		t.Errorf("got %d members after a leave, want 1", len(members.Msg.GetMembers()))
	}

	_, err = second.ListMessages(ctx, connect.NewRequest(&chatv1.ListMessagesRequest{ChannelId: channelID}))
	if code := codeOf(t, err); code != connect.CodePermissionDenied {
		t.Errorf("reading after leaving = %s, want permission_denied", code)
	}
}

func TestSendMessageIsIdempotentOnClientMessageID(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	client := h.chatClient(t, "rill")
	ctx := t.Context()

	channelID := h.joinChannel(t, client, "teste")
	clientID := h.newID(t)

	first, err := client.SendMessage(ctx, connect.NewRequest(&chatv1.SendMessageRequest{
		ChannelId:       channelID,
		ClientMessageId: clientID,
		Body:            "anyone here actually testing",
		Kind:            chatv1.MessageKind_MESSAGE_KIND_NORMAL,
	}))
	if err != nil {
		t.Fatalf("first send: %v", err)
	}
	if first.Msg.GetDeduplicated() {
		t.Error("first send reported deduplicated")
	}

	retry, err := client.SendMessage(ctx, connect.NewRequest(&chatv1.SendMessageRequest{
		ChannelId:       channelID,
		ClientMessageId: clientID,
		Body:            "anyone here actually testing",
		Kind:            chatv1.MessageKind_MESSAGE_KIND_NORMAL,
	}))
	if err != nil {
		t.Fatalf("retry: %v", err)
	}
	if !retry.Msg.GetDeduplicated() {
		t.Error("retry did not report deduplicated")
	}
	if retry.Msg.GetMessage().GetId() != first.Msg.GetMessage().GetId() {
		t.Error("retry created a new message instead of returning the original")
	}

	page, err := client.ListMessages(ctx, connect.NewRequest(&chatv1.ListMessagesRequest{ChannelId: channelID}))
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	if got := countKind(page.Msg.GetMessages(), chatv1.MessageKind_MESSAGE_KIND_NORMAL); got != 1 {
		t.Errorf("channel holds %d normal messages, want 1", got)
	}
}

func TestSendMessageRejectsServerOnlyKinds(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	client := h.chatClient(t, "rill")
	ctx := t.Context()

	channelID := h.joinChannel(t, client, "teste")

	tests := []struct {
		name string
		kind chatv1.MessageKind
	}{
		{name: "system", kind: chatv1.MessageKind_MESSAGE_KIND_SYSTEM},
		{name: "direct", kind: chatv1.MessageKind_MESSAGE_KIND_DIRECT},
		{name: "unspecified", kind: chatv1.MessageKind_MESSAGE_KIND_UNSPECIFIED},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := client.SendMessage(ctx, connect.NewRequest(&chatv1.SendMessageRequest{
				ChannelId:       channelID,
				ClientMessageId: h.newID(t),
				Body:            "impostor",
				Kind:            tc.kind,
			}))
			if code := codeOf(t, err); code != connect.CodeInvalidArgument {
				t.Errorf("code = %s, want invalid_argument", code)
			}
			if reason := detailReason(t, err); reason != chatv1.ErrorReason_ERROR_REASON_MESSAGE_KIND_INVALID {
				t.Errorf("reason = %s, want MESSAGE_KIND_INVALID", reason)
			}
		})
	}
}

func TestSendMessageRejectsANonMember(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	member := h.chatClient(t, "rill")
	stranger := h.chatClient(t, "anon2")
	ctx := t.Context()

	channelID := h.joinChannel(t, member, "teste")

	_, err := stranger.SendMessage(ctx, connect.NewRequest(&chatv1.SendMessageRequest{
		ChannelId:       channelID,
		ClientMessageId: h.newID(t),
		Body:            "let me in",
		Kind:            chatv1.MessageKind_MESSAGE_KIND_NORMAL,
	}))
	if code := codeOf(t, err); code != connect.CodePermissionDenied {
		t.Errorf("code = %s, want permission_denied", code)
	}
	if reason := detailReason(t, err); reason != chatv1.ErrorReason_ERROR_REASON_NOT_A_MEMBER {
		t.Errorf("reason = %s, want NOT_A_MEMBER", reason)
	}
}

func TestSendMessageValidationCarriesATypedDetail(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	client := h.chatClient(t, "rill")
	ctx := t.Context()

	channelID := h.joinChannel(t, client, "teste")

	tests := []struct {
		name     string
		body     string
		clientID string
		reason   chatv1.ErrorReason
	}{
		{name: "empty body", body: "", clientID: h.newID(t), reason: chatv1.ErrorReason_ERROR_REASON_BODY_EMPTY},
		{name: "whitespace body", body: "  \n ", clientID: h.newID(t), reason: chatv1.ErrorReason_ERROR_REASON_BODY_EMPTY},
		{
			name:     "body too long",
			body:     strings.Repeat("a", 2001),
			clientID: h.newID(t),
			reason:   chatv1.ErrorReason_ERROR_REASON_BODY_TOO_LONG,
		},
		{
			name:     "client id missing",
			body:     "hello",
			clientID: "",
			reason:   chatv1.ErrorReason_ERROR_REASON_CLIENT_MESSAGE_ID_INVALID,
		},
		{
			name:     "client id not a ulid",
			body:     "hello",
			clientID: "not-a-ulid",
			reason:   chatv1.ErrorReason_ERROR_REASON_CLIENT_MESSAGE_ID_INVALID,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := client.SendMessage(ctx, connect.NewRequest(&chatv1.SendMessageRequest{
				ChannelId:       channelID,
				ClientMessageId: tc.clientID,
				Body:            tc.body,
				Kind:            chatv1.MessageKind_MESSAGE_KIND_NORMAL,
			}))
			if code := codeOf(t, err); code != connect.CodeInvalidArgument {
				t.Errorf("code = %s, want invalid_argument", code)
			}
			if reason := detailReason(t, err); reason != tc.reason {
				t.Errorf("reason = %s, want %s", reason, tc.reason)
			}
		})
	}
}

func TestBodyTooLongReportsTheLimit(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	client := h.chatClient(t, "rill")
	ctx := t.Context()

	channelID := h.joinChannel(t, client, "teste")

	_, err := client.SendMessage(ctx, connect.NewRequest(&chatv1.SendMessageRequest{
		ChannelId:       channelID,
		ClientMessageId: h.newID(t),
		Body:            strings.Repeat("a", 5000),
		Kind:            chatv1.MessageKind_MESSAGE_KIND_NORMAL,
	}))
	if got := detailLimit(t, err); got != uint32(chat.DefaultLimits().BodyMaxLen) {
		t.Errorf("limit = %d, want %d", got, chat.DefaultLimits().BodyMaxLen)
	}
}

func TestListMessagesPaginatesNewestFirst(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	client := h.chatClient(t, "rill")
	ctx := t.Context()

	channelID := h.joinChannel(t, client, "teste")

	const sent = 6
	for i := range sent {
		h.send(t, client, channelID, string(rune('a'+i)))
	}

	first, err := client.ListMessages(ctx, connect.NewRequest(&chatv1.ListMessagesRequest{
		ChannelId: channelID,
		PageSize:  3,
	}))
	if err != nil {
		t.Fatalf("first page: %v", err)
	}
	if got := bodiesOf(first.Msg.GetMessages()); !equalStrings(got, []string{"f", "e", "d"}) {
		t.Errorf("first page = %v, want [f e d]", got)
	}
	if first.Msg.GetNextCursor().GetValue() == "" {
		t.Fatal("first page carries no cursor")
	}

	second, err := client.ListMessages(ctx, connect.NewRequest(&chatv1.ListMessagesRequest{
		ChannelId: channelID,
		PageSize:  3,
		Cursor:    first.Msg.GetNextCursor(),
	}))
	if err != nil {
		t.Fatalf("second page: %v", err)
	}
	if got := bodiesOf(second.Msg.GetMessages()); !equalStrings(got, []string{"c", "b", "a"}) {
		t.Errorf("second page = %v, want [c b a]", got)
	}
}

func TestListMessagesReportsAnEvictedWindow(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	client := h.chatClient(t, "rill")
	ctx := t.Context()

	channelID := h.joinChannel(t, client, "teste")

	for i := range h.capacity + 4 {
		h.send(t, client, channelID, "m"+string(rune('a'+i)))
	}

	page, err := client.ListMessages(ctx, connect.NewRequest(&chatv1.ListMessagesRequest{
		ChannelId: channelID,
		PageSize:  uint32(h.capacity),
	}))
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	if !page.Msg.GetWindowTruncated() {
		t.Error("window_truncated is false after eviction")
	}
	if len(page.Msg.GetMessages()) != h.capacity {
		t.Errorf("got %d messages, want the capacity %d", len(page.Msg.GetMessages()), h.capacity)
	}
}

func TestListMessagesRejectsAMalformedCursor(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	client := h.chatClient(t, "rill")
	ctx := t.Context()

	channelID := h.joinChannel(t, client, "teste")

	_, err := client.ListMessages(ctx, connect.NewRequest(&chatv1.ListMessagesRequest{
		ChannelId: channelID,
		Cursor:    &chatv1.Cursor{Value: "nonsense"},
	}))
	if code := codeOf(t, err); code != connect.CodeInvalidArgument {
		t.Errorf("code = %s, want invalid_argument", code)
	}
	if reason := detailReason(t, err); reason != chatv1.ErrorReason_ERROR_REASON_CURSOR_INVALID {
		t.Errorf("reason = %s, want CURSOR_INVALID", reason)
	}
}

func TestSendMessageIsRateLimitedPerSession(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	client := h.chatClient(t, "rill")
	other := h.chatClient(t, "anon2")
	ctx := t.Context()

	channelID := h.joinChannel(t, client, "teste")
	if _, err := other.JoinChannel(ctx, connect.NewRequest(&chatv1.JoinChannelRequest{Name: "teste"})); err != nil {
		t.Fatalf("second join: %v", err)
	}

	var limited error
	for range h.burst + 5 {
		_, err := client.SendMessage(ctx, connect.NewRequest(&chatv1.SendMessageRequest{
			ChannelId:       channelID,
			ClientMessageId: h.newID(t),
			Body:            "flood",
			Kind:            chatv1.MessageKind_MESSAGE_KIND_NORMAL,
		}))
		if err != nil {
			limited = err
			break
		}
	}

	if limited == nil {
		t.Fatal("the limiter never fired")
	}
	if code := codeOf(t, limited); code != connect.CodeResourceExhausted {
		t.Errorf("code = %s, want resource_exhausted", code)
	}
	if reason := detailReason(t, limited); reason != chatv1.ErrorReason_ERROR_REASON_RATE_LIMITED {
		t.Errorf("reason = %s, want RATE_LIMITED", reason)
	}
	if retry := detailRetryAfter(t, limited); retry <= 0 {
		t.Errorf("retry_after = %s, want a positive delay", retry)
	}

	if _, err := other.SendMessage(ctx, connect.NewRequest(&chatv1.SendMessageRequest{
		ChannelId:       channelID,
		ClientMessageId: h.newID(t),
		Body:            "unaffected",
		Kind:            chatv1.MessageKind_MESSAGE_KIND_NORMAL,
	})); err != nil {
		t.Errorf("a second session was caught by another session's limit: %v", err)
	}
}

func TestRateLimitRecoversAsTokensRefill(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	client := h.chatClient(t, "rill")
	ctx := t.Context()

	channelID := h.joinChannel(t, client, "teste")

	for range h.burst + 2 {
		if _, floodErr := client.SendMessage(ctx, connect.NewRequest(&chatv1.SendMessageRequest{
			ChannelId:       channelID,
			ClientMessageId: h.newID(t),
			Body:            "flood",
			Kind:            chatv1.MessageKind_MESSAGE_KIND_NORMAL,
		})); floodErr != nil {
			break
		}
	}

	h.clock.Advance(time.Minute)

	if _, err := client.SendMessage(ctx, connect.NewRequest(&chatv1.SendMessageRequest{
		ChannelId:       channelID,
		ClientMessageId: h.newID(t),
		Body:            "recovered",
		Kind:            chatv1.MessageKind_MESSAGE_KIND_NORMAL,
	})); err != nil {
		t.Errorf("still limited after the refill window: %v", err)
	}
}

func TestSetTypingRequiresMembership(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	member := h.chatClient(t, "rill")
	stranger := h.chatClient(t, "anon2")
	ctx := t.Context()

	channelID := h.joinChannel(t, member, "teste")

	if _, err := member.SetTyping(ctx, connect.NewRequest(&chatv1.SetTypingRequest{
		ChannelId: channelID,
		Typing:    true,
	})); err != nil {
		t.Fatalf("SetTyping: %v", err)
	}

	members, err := member.ListMembers(ctx, connect.NewRequest(&chatv1.ListMembersRequest{ChannelId: channelID}))
	if err != nil {
		t.Fatalf("ListMembers: %v", err)
	}
	if !members.Msg.GetMembers()[0].GetTyping() {
		t.Error("member is not marked as typing")
	}

	_, err = stranger.SetTyping(ctx, connect.NewRequest(&chatv1.SetTypingRequest{
		ChannelId: channelID,
		Typing:    true,
	}))
	if code := codeOf(t, err); code != connect.CodePermissionDenied {
		t.Errorf("stranger SetTyping = %s, want permission_denied", code)
	}
}

func TestTypingExpiresOnItsOwn(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	client := h.chatClient(t, "rill")
	ctx := t.Context()

	channelID := h.joinChannel(t, client, "teste")

	if _, err := client.SetTyping(ctx, connect.NewRequest(&chatv1.SetTypingRequest{
		ChannelId: channelID,
		Typing:    true,
	})); err != nil {
		t.Fatalf("SetTyping: %v", err)
	}

	h.clock.Advance(h.typingTTL + time.Second)

	members, err := client.ListMembers(ctx, connect.NewRequest(&chatv1.ListMembersRequest{ChannelId: channelID}))
	if err != nil {
		t.Fatalf("ListMembers: %v", err)
	}
	if members.Msg.GetMembers()[0].GetTyping() {
		t.Error("typing survived its TTL, the client would have to time it out itself")
	}
}

func countKind(messages []*chatv1.Message, kind chatv1.MessageKind) int {
	n := 0
	for _, message := range messages {
		if message.GetKind() == kind {
			n++
		}
	}
	return n
}

func bodiesOf(messages []*chatv1.Message) []string {
	out := make([]string, len(messages))
	for i, message := range messages {
		out[i] = message.GetBody()
	}
	return out
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
