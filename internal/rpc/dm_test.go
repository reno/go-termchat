package rpc_test

import (
	"testing"
	"time"

	"connectrpc.com/connect"

	chatv1 "github.com/reno/go-termchat/gen/go/chat/v1"
	"github.com/reno/go-termchat/internal/chat"
	"github.com/reno/go-termchat/internal/hub"
)

func TestDirectMessageReachesOnlyTheTwoParties(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	sender := h.chatClient(t, "rill")
	ctx := t.Context()

	senderSub := h.subscribe(t, "rill")
	recipientSub := h.subscribe(t, "0x9c")
	bystanderSub := h.subscribe(t, "anon2")

	res, err := sender.SendDirectMessage(ctx, connect.NewRequest(&chatv1.SendDirectMessageRequest{
		ClientMessageId: h.newID(t),
		RecipientNick:   "0x9c",
		Body:            "the void, mostly",
	}))
	if err != nil {
		t.Fatalf("SendDirectMessage: %v", err)
	}
	if !res.Msg.GetDelivered() {
		t.Fatal("delivered is false for a connected recipient")
	}
	if kind := res.Msg.GetMessage().GetKind(); kind != chatv1.MessageKind_MESSAGE_KIND_DIRECT {
		t.Errorf("kind = %s, want DIRECT", kind)
	}

	got := receiveEvent(t, recipientSub)
	if got.Kind != chat.EventDirectMessage {
		t.Errorf("recipient got %s, want direct_message", got.Kind)
	}
	if got.Message.Body != "the void, mostly" {
		t.Errorf("recipient body = %q", got.Message.Body)
	}
	if got.Recipient != "0x9c" {
		t.Errorf("recipient field = %q", got.Recipient)
	}

	if echo := receiveEvent(t, senderSub); echo.Kind != chat.EventDirectMessage {
		t.Errorf("sender echo = %s, want direct_message", echo.Kind)
	}

	expectNoEvent(t, bystanderSub)
}

func TestDirectMessageToAnOfflineNickIsNotDelivered(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	sender := h.chatClient(t, "rill")
	ctx := t.Context()

	res, err := sender.SendDirectMessage(ctx, connect.NewRequest(&chatv1.SendDirectMessageRequest{
		ClientMessageId: h.newID(t),
		RecipientNick:   "ghost",
		Body:            "anyone there",
	}))
	if err != nil {
		t.Fatalf("SendDirectMessage: %v", err)
	}
	if res.Msg.GetDelivered() {
		t.Error("delivered is true for a nick with no session")
	}
}

func TestDirectMessageToAJoinedButUnsubscribedNickIsNotDelivered(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	sender := h.chatClient(t, "rill")
	_ = h.chatClient(t, "0x9c")
	ctx := t.Context()

	res, err := sender.SendDirectMessage(ctx, connect.NewRequest(&chatv1.SendDirectMessageRequest{
		ClientMessageId: h.newID(t),
		RecipientNick:   "0x9c",
		Body:            "psst",
	}))
	if err != nil {
		t.Fatalf("SendDirectMessage: %v", err)
	}
	if res.Msg.GetDelivered() {
		t.Error("delivered is true for a session holding no live stream")
	}
}

func TestDirectMessagesAreNotRetained(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	sender := h.chatClient(t, "rill")
	ctx := t.Context()

	h.subscribe(t, "0x9c")
	channelID := h.joinChannel(t, sender, "teste")

	if _, err := sender.SendDirectMessage(ctx, connect.NewRequest(&chatv1.SendDirectMessageRequest{
		ClientMessageId: h.newID(t),
		RecipientNick:   "0x9c",
		Body:            "off the record",
	})); err != nil {
		t.Fatalf("SendDirectMessage: %v", err)
	}

	page, err := sender.ListMessages(ctx, connect.NewRequest(&chatv1.ListMessagesRequest{
		ChannelId: channelID,
	}))
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	for _, message := range page.Msg.GetMessages() {
		if message.GetKind() == chatv1.MessageKind_MESSAGE_KIND_DIRECT {
			t.Error("a direct message was retained in a channel transcript")
		}
		if message.GetBody() == "off the record" {
			t.Error("direct message body reachable through ListMessages")
		}
	}
}

func TestDirectMessageValidatesItsInputs(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	sender := h.chatClient(t, "rill")
	ctx := t.Context()

	tests := []struct {
		name      string
		recipient string
		clientID  string
		body      string
		reason    chatv1.ErrorReason
	}{
		{
			name:      "invalid nick",
			recipient: "not a nick",
			clientID:  h.newID(t),
			body:      "hi",
			reason:    chatv1.ErrorReason_ERROR_REASON_NICK_INVALID,
		},
		{
			name:      "empty body",
			recipient: "0x9c",
			clientID:  h.newID(t),
			body:      "   ",
			reason:    chatv1.ErrorReason_ERROR_REASON_BODY_EMPTY,
		},
		{
			name:      "bad client id",
			recipient: "0x9c",
			clientID:  "nope",
			body:      "hi",
			reason:    chatv1.ErrorReason_ERROR_REASON_CLIENT_MESSAGE_ID_INVALID,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := sender.SendDirectMessage(ctx, connect.NewRequest(&chatv1.SendDirectMessageRequest{
				ClientMessageId: tc.clientID,
				RecipientNick:   tc.recipient,
				Body:            tc.body,
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

func TestChannelMessagesReachEveryMemberButNotOutsiders(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	author := h.chatClient(t, "rill")

	channelID := h.joinChannel(t, author, "teste")

	memberSub := h.subscribeToChannel(t, "0x9c", channelID)
	outsiderSub := h.subscribe(t, "anon2")

	h.send(t, author, channelID, "anyone here actually testing")

	got := receiveEvent(t, memberSub)
	if got.Kind != chat.EventMessageCreated {
		t.Errorf("member got %s, want message_created", got.Kind)
	}
	if got.Message.Body != "anyone here actually testing" {
		t.Errorf("body = %q", got.Message.Body)
	}

	expectNoEvent(t, outsiderSub)
}

func TestADeduplicatedSendPublishesNothingASecondTime(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	author := h.chatClient(t, "rill")
	ctx := t.Context()

	channelID := h.joinChannel(t, author, "teste")
	watcher := h.subscribeToChannel(t, "0x9c", channelID)

	clientID := h.newID(t)
	request := &chatv1.SendMessageRequest{
		ChannelId:       channelID,
		ClientMessageId: clientID,
		Body:            "once",
		Kind:            chatv1.MessageKind_MESSAGE_KIND_NORMAL,
	}

	if _, err := author.SendMessage(ctx, connect.NewRequest(request)); err != nil {
		t.Fatalf("first send: %v", err)
	}
	if got := receiveEvent(t, watcher); got.Message.Body != "once" {
		t.Fatalf("first event body = %q", got.Message.Body)
	}

	h.clock.Advance(h.refill)
	if _, err := author.SendMessage(ctx, connect.NewRequest(request)); err != nil {
		t.Fatalf("retry: %v", err)
	}

	expectNoEvent(t, watcher)
}

func receiveEvent(t *testing.T, sub *hub.Subscription) chat.Event {
	t.Helper()

	select {
	case got := <-sub.Events():
		return got
	case <-time.After(2 * time.Second):
		t.Fatal("no event arrived")
		return chat.Event{}
	}
}

func expectNoEvent(t *testing.T, sub *hub.Subscription) {
	t.Helper()

	select {
	case got := <-sub.Events():
		t.Fatalf("unexpected %s event reached a party that should not see it", got.Kind)
	case <-time.After(100 * time.Millisecond):
	}
}
