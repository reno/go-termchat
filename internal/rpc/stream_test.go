package rpc_test

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"

	chatv1 "github.com/reno/go-termchat/gen/go/chat/v1"
	"github.com/reno/go-termchat/gen/go/chat/v1/chatv1connect"
	"github.com/reno/go-termchat/internal/rpc"
)

type liveStream struct {
	cancel context.CancelFunc
	frames chan *chatv1.StreamEventsResponse
	done   chan error
}

func (h *harness) stream(t *testing.T, client chatv1connect.ChatServiceClient, lastEventID string) *liveStream {
	t.Helper()

	ctx, cancel := context.WithCancel(t.Context())
	live := &liveStream{
		cancel: cancel,
		frames: make(chan *chatv1.StreamEventsResponse, 64),
		done:   make(chan error, 1),
	}

	before := h.hub.Subscribers()
	opened := make(chan struct{})
	go func() {
		defer close(live.done)

		res, err := client.StreamEvents(ctx, connect.NewRequest(&chatv1.StreamEventsRequest{
			LastEventId: lastEventID,
		}))
		close(opened)
		if err != nil {
			live.done <- err
			return
		}
		defer func() { _ = res.Close() }()

		for res.Receive() {
			select {
			case live.frames <- res.Msg():
			default:
			}
		}
		live.done <- res.Err()
	}()

	<-opened
	t.Cleanup(func() {
		cancel()
		<-live.done
	})

	waitFor(t, func() bool { return h.hub.Subscribers() > before },
		"the server to register the subscription")

	return live
}

func (l *liveStream) next(t *testing.T, within time.Duration) *chatv1.Event {
	t.Helper()

	deadline := time.After(within)
	for {
		select {
		case frame := <-l.frames:
			if frame.GetEvent() == nil {
				continue
			}
			return frame.GetEvent()
		case <-deadline:
			t.Fatal("no event arrived on the stream")
			return nil
		}
	}
}

func (l *liveStream) quiet(t *testing.T, within time.Duration) {
	t.Helper()

	deadline := time.After(within)
	for {
		select {
		case frame := <-l.frames:
			created := frame.GetEvent().GetMessageCreated()
			if created == nil {
				continue
			}
			t.Fatalf("unexpected message %q", created.GetMessage().GetBody())
		case <-deadline:
			return
		}
	}
}

func (l *liveStream) close(t *testing.T) {
	t.Helper()

	l.cancel()
	select {
	case <-l.done:
	case <-time.After(3 * time.Second):
		t.Fatal("stream did not shut down after cancellation")
	}
}

func TestStreamEventsDeliversChannelMessagesLive(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	author := h.chatClient(t, "rill")
	watcher := h.chatClient(t, "0x9c")

	channelID := h.joinChannel(t, author, "teste")
	if _, err := watcher.JoinChannel(t.Context(), connect.NewRequest(&chatv1.JoinChannelRequest{
		Name: "teste",
	})); err != nil {
		t.Fatalf("watcher join: %v", err)
	}

	live := h.stream(t, watcher, "")
	h.send(t, author, channelID, "anyone here actually testing")

	for {
		event := live.next(t, 3*time.Second)
		created := event.GetMessageCreated()
		if created == nil {
			continue
		}
		if created.GetMessage().GetBody() == "anyone here actually testing" {
			return
		}
	}
}

func TestStreamEventsSendsKeepalives(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	client := h.chatClient(t, "rill")

	live := h.stream(t, client, "")

	select {
	case frame := <-live.frames:
		if frame.GetEvent() != nil {
			t.Fatalf("expected a keepalive, got event %s", frame.GetEvent().GetEventId())
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no keepalive arrived")
	}
}

func TestStreamEventsReleasesItsSubscriptionOnDisconnect(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	client := h.chatClient(t, "rill")

	live := h.stream(t, client, "")

	waitFor(t, func() bool { return h.hub.Subscribers() == 1 }, "subscriber count to reach 1")

	live.close(t)

	waitFor(t, func() bool { return h.hub.Subscribers() == 0 }, "subscriber count to return to 0")
}

func TestManyStreamsAllReleaseTheirSubscriptions(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	const clients = 10

	streams := make([]*liveStream, 0, clients)
	for i := range clients {
		client := h.chatClient(t, "user"+string(rune('a'+i)))
		streams = append(streams, h.stream(t, client, ""))
	}

	waitFor(t, func() bool { return h.hub.Subscribers() == clients }, "every stream to subscribe")

	for _, live := range streams {
		live.close(t)
	}

	waitFor(t, func() bool { return h.hub.Subscribers() == 0 }, "every subscription to be released")
}

func TestStreamEventsAnnouncesPresence(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	watcher := h.chatClient(t, "rill")
	joiner := h.chatClient(t, "0x9c")

	channelID := h.joinChannel(t, watcher, "teste")
	if _, err := joiner.JoinChannel(t.Context(), connect.NewRequest(&chatv1.JoinChannelRequest{
		Name: "teste",
	})); err != nil {
		t.Fatalf("joiner join: %v", err)
	}

	watching := h.stream(t, watcher, "")
	joining := h.stream(t, joiner, "")

	found := false
	for range 10 {
		event := watching.next(t, 3*time.Second)
		presence := event.GetPresenceChanged()
		if presence == nil {
			continue
		}
		if presence.GetNick() == "0x9c" && presence.GetPresence() == chatv1.Presence_PRESENCE_ACTIVE {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("no active presence event for the joining member")
	}

	joining.close(t)

	for range 10 {
		event := watching.next(t, 3*time.Second)
		presence := event.GetPresenceChanged()
		if presence == nil {
			continue
		}
		if presence.GetNick() == "0x9c" && presence.GetPresence() == chatv1.Presence_PRESENCE_IDLE {
			_ = channelID
			return
		}
	}
	t.Fatal("no idle presence event after the member's stream dropped")
}

func TestResumeReplaysEveryMissedMessageExactlyOnce(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	author := h.chatClient(t, "rill")
	reader := h.chatClient(t, "0x9c")

	channelID := h.joinChannel(t, author, "teste")
	if _, err := reader.JoinChannel(t.Context(), connect.NewRequest(&chatv1.JoinChannelRequest{
		Name: "teste",
	})); err != nil {
		t.Fatalf("reader join: %v", err)
	}

	live := h.stream(t, reader, "")
	first := h.send(t, author, channelID, "before the drop")

	waitForBody(t, live, "before the drop")
	live.close(t)

	const missed = 5
	for i := range missed {
		h.send(t, author, channelID, "missed-"+string(rune('a'+i)))
	}

	resumed := h.stream(t, reader, first.GetId())

	seen := make(map[string]int)
	for len(seen) < missed {
		event := resumed.next(t, 5*time.Second)
		created := event.GetMessageCreated()
		if created == nil {
			continue
		}
		body := created.GetMessage().GetBody()
		if body == "before the drop" {
			t.Fatal("resume replayed a message the client had already seen")
		}
		seen[body]++
	}

	resumed.quiet(t, 300*time.Millisecond)

	for i := range missed {
		body := "missed-" + string(rune('a'+i))
		if seen[body] != 1 {
			t.Errorf("%q arrived %d times, want exactly 1", body, seen[body])
		}
	}
}

func TestResumeTellsTheClientWhenHistoryWasForgotten(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	author := h.chatClient(t, "rill")
	reader := h.chatClient(t, "0x9c")

	channelID := h.joinChannel(t, author, "teste")
	if _, err := reader.JoinChannel(t.Context(), connect.NewRequest(&chatv1.JoinChannelRequest{
		Name: "teste",
	})); err != nil {
		t.Fatalf("reader join: %v", err)
	}

	anchor := h.send(t, author, channelID, "anchor")

	for i := range h.capacity + 4 {
		h.send(t, author, channelID, "flood-"+string(rune('a'+i)))
	}

	resumed := h.stream(t, reader, anchor.GetId())

	for range 20 {
		event := resumed.next(t, 5*time.Second)
		created := event.GetMessageCreated()
		if created == nil {
			continue
		}
		if created.GetMessage().GetBody() == rpc.ForgottenNotice {
			if kind := created.GetMessage().GetKind(); kind != chatv1.MessageKind_MESSAGE_KIND_SYSTEM {
				t.Errorf("notice kind = %s, want SYSTEM", kind)
			}
			return
		}
	}
	t.Fatalf("no %q system line after the window evicted underneath the cursor", rpc.ForgottenNotice)
}

func TestResumeRejectsAMalformedLastEventID(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	client := h.chatClient(t, "rill")

	res, err := client.StreamEvents(t.Context(), connect.NewRequest(&chatv1.StreamEventsRequest{
		LastEventId: "not-a-ulid",
	}))
	if err == nil {
		for res.Receive() {
		}
		err = res.Err()
		_ = res.Close()
	}
	if code := codeOf(t, err); code != connect.CodeInvalidArgument {
		t.Errorf("code = %s, want invalid_argument", code)
	}
}

func TestStreamEventsRequiresASession(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	client := chatv1connect.NewChatServiceClient(h.httpClient(t, nil), h.server.URL)

	res, err := client.StreamEvents(t.Context(), connect.NewRequest(&chatv1.StreamEventsRequest{}))
	if err == nil {
		for res.Receive() {
		}
		err = res.Err()
		_ = res.Close()
	}
	if code := codeOf(t, err); code != connect.CodeUnauthenticated {
		t.Errorf("code = %s, want unauthenticated", code)
	}
}

func waitForBody(t *testing.T, live *liveStream, body string) {
	t.Helper()

	for range 20 {
		event := live.next(t, 3*time.Second)
		created := event.GetMessageCreated()
		if created != nil && created.GetMessage().GetBody() == body {
			return
		}
	}
	t.Fatalf("never saw %q on the stream", body)
}

func waitFor(t *testing.T, condition func() bool, what string) {
	t.Helper()

	deadline := time.After(5 * time.Second)
	for {
		if condition() {
			return
		}
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for %s", what)
		case <-time.After(10 * time.Millisecond):
		}
	}
}
