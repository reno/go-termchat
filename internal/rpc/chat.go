package rpc

import (
	"context"
	"time"

	"connectrpc.com/connect"

	chatv1 "github.com/reno/go-termchat/gen/go/chat/v1"
	"github.com/reno/go-termchat/gen/go/chat/v1/chatv1connect"
	"github.com/reno/go-termchat/internal/chat"
	"github.com/reno/go-termchat/internal/hub"
	"github.com/reno/go-termchat/internal/store"
)

const CreatedChannelSuffix = " — you created this channel."

type ChatHandler struct {
	store   store.Store
	hub     *hub.Hub
	rules   chat.Rules
	limiter *chat.RateLimiter
	ids     *chat.IDGen
	clock   store.Clock
	typing  time.Duration
}

var _ chatv1connect.ChatServiceHandler = (*ChatHandler)(nil)

type ChatOption func(*ChatHandler)

func WithChatClock(c store.Clock) ChatOption {
	return func(h *ChatHandler) { h.clock = c }
}

func NewChatHandler(
	st store.Store,
	events *hub.Hub,
	rules chat.Rules,
	limiter *chat.RateLimiter,
	ids *chat.IDGen,
	typingTTL time.Duration,
	opts ...ChatOption,
) *ChatHandler {
	h := &ChatHandler{
		store:   st,
		hub:     events,
		rules:   rules,
		limiter: limiter,
		ids:     ids,
		clock:   store.SystemClock{},
		typing:  typingTTL,
	}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

func (h *ChatHandler) ListChannels(
	ctx context.Context,
	_ *connect.Request[chatv1.ListChannelsRequest],
) (*connect.Response[chatv1.ListChannelsResponse], error) {
	if _, err := caller(ctx); err != nil {
		return nil, err
	}

	channels, err := h.store.Channels(ctx)
	if err != nil {
		return nil, Error(err)
	}

	out := make([]*chatv1.Channel, 0, len(channels))
	for _, channel := range channels {
		members, membersErr := h.store.Members(ctx, channel.ID)
		if membersErr != nil {
			return nil, Error(membersErr)
		}
		out = append(out, ChannelProto(channel, len(members)))
	}
	return connect.NewResponse(&chatv1.ListChannelsResponse{Channels: out}), nil
}

func (h *ChatHandler) JoinChannel(
	ctx context.Context,
	req *connect.Request[chatv1.JoinChannelRequest],
) (*connect.Response[chatv1.JoinChannelResponse], error) {
	current, err := caller(ctx)
	if err != nil {
		return nil, err
	}

	name, err := h.rules.NormalizeChannelName(req.Msg.GetName())
	if err != nil {
		return nil, Error(err)
	}

	now := h.clock.Now()
	channelID, err := h.ids.New(now)
	if err != nil {
		return nil, Error(err)
	}

	channel, created, err := h.store.EnsureChannel(ctx, channelID, name, now)
	if err != nil {
		return nil, Error(err)
	}

	member := chat.Member{
		SessionID: current.ID,
		Nick:      current.Nick,
		JoinedAt:  now,
		Presence:  chat.PresenceActive,
	}
	if joinErr := h.store.JoinChannel(ctx, channel.ID, member); joinErr != nil {
		return nil, Error(joinErr)
	}
	h.hub.JoinChannel(current.ID, channel.ID)

	if created {
		if seedErr := h.seedCreationNotice(ctx, channel, now); seedErr != nil {
			return nil, Error(seedErr)
		}
	}

	if publishErr := h.publishChannel(channel.ID, now, func(id string, at time.Time) chat.Event {
		return chat.MemberJoined(id, at, channel.ID, member)
	}); publishErr != nil {
		return nil, Error(publishErr)
	}

	members, err := h.store.Members(ctx, channel.ID)
	if err != nil {
		return nil, Error(err)
	}

	return connect.NewResponse(&chatv1.JoinChannelResponse{
		Channel: ChannelProto(channel, len(members)),
		Created: created,
	}), nil
}

func (h *ChatHandler) LeaveChannel(
	ctx context.Context,
	req *connect.Request[chatv1.LeaveChannelRequest],
) (*connect.Response[chatv1.LeaveChannelResponse], error) {
	current, err := caller(ctx)
	if err != nil {
		return nil, err
	}

	channelID := req.Msg.GetChannelId()
	if err := h.store.LeaveChannel(ctx, channelID, current.ID); err != nil {
		return nil, Error(err)
	}
	h.hub.LeaveChannel(current.ID, channelID)

	now := h.clock.Now()
	if err := h.publishChannel(channelID, now, func(id string, at time.Time) chat.Event {
		return chat.MemberLeft(id, at, channelID, current.Nick)
	}); err != nil {
		return nil, Error(err)
	}

	return connect.NewResponse(&chatv1.LeaveChannelResponse{}), nil
}

func (h *ChatHandler) ListMembers(
	ctx context.Context,
	req *connect.Request[chatv1.ListMembersRequest],
) (*connect.Response[chatv1.ListMembersResponse], error) {
	current, err := caller(ctx)
	if err != nil {
		return nil, err
	}

	channelID := req.Msg.GetChannelId()
	if membershipErr := h.requireMembership(ctx, channelID, current.ID); membershipErr != nil {
		return nil, Error(membershipErr)
	}

	members, err := h.store.Members(ctx, channelID)
	if err != nil {
		return nil, Error(err)
	}

	now := h.clock.Now()
	out := make([]*chatv1.Member, 0, len(members))
	for _, member := range members {
		out = append(out, MemberProto(member, now))
	}
	return connect.NewResponse(&chatv1.ListMembersResponse{Members: out}), nil
}

func (h *ChatHandler) ListMessages(
	ctx context.Context,
	req *connect.Request[chatv1.ListMessagesRequest],
) (*connect.Response[chatv1.ListMessagesResponse], error) {
	current, err := caller(ctx)
	if err != nil {
		return nil, err
	}

	channelID := req.Msg.GetChannelId()
	if membershipErr := h.requireMembership(ctx, channelID, current.ID); membershipErr != nil {
		return nil, Error(membershipErr)
	}

	page, err := h.store.Messages(ctx, channelID, req.Msg.GetCursor().GetValue(), int(req.Msg.GetPageSize()))
	if err != nil {
		return nil, Error(err)
	}

	out := make([]*chatv1.Message, 0, len(page.Messages))
	for _, message := range page.Messages {
		out = append(out, MessageProto(message))
	}

	res := &chatv1.ListMessagesResponse{
		Messages:        out,
		WindowTruncated: page.Truncated,
	}
	if page.Next != "" {
		res.NextCursor = &chatv1.Cursor{Value: page.Next}
	}
	return connect.NewResponse(res), nil
}

func (h *ChatHandler) SendMessage(
	ctx context.Context,
	req *connect.Request[chatv1.SendMessageRequest],
) (*connect.Response[chatv1.SendMessageResponse], error) {
	current, err := caller(ctx)
	if err != nil {
		return nil, err
	}

	channelID := req.Msg.GetChannelId()
	member, err := h.store.IsMember(ctx, channelID, current.ID)
	if err != nil {
		return nil, Error(err)
	}

	kind := KindFrom(req.Msg.GetKind())
	if postErr := h.rules.AuthorizePost(member, kind); postErr != nil {
		return nil, Error(postErr)
	}

	clientID := req.Msg.GetClientMessageId()
	if !chat.ValidID(clientID) {
		return nil, Error(chat.Fail(chat.ReasonClientMessageIDInvalid, "client_message_id"))
	}

	body, err := h.rules.NormalizeBody(req.Msg.GetBody())
	if err != nil {
		return nil, Error(err)
	}

	now := h.clock.Now()
	if rateErr := h.limiter.Allow(current.ID, now); rateErr != nil {
		return nil, Error(rateErr)
	}

	id, err := h.ids.New(now)
	if err != nil {
		return nil, Error(err)
	}

	stored, deduplicated, err := h.store.Append(ctx, chat.Message{
		ID:        id,
		ClientID:  clientID,
		ChannelID: channelID,
		Author:    current.Nick,
		Body:      body,
		Kind:      kind,
		CreatedAt: now,
	})
	if err != nil {
		return nil, Error(err)
	}

	if !deduplicated {
		if err := h.publishChannel(channelID, now, func(eventID string, at time.Time) chat.Event {
			return chat.MessageCreated(eventID, at, stored)
		}); err != nil {
			return nil, Error(err)
		}
	}

	return connect.NewResponse(&chatv1.SendMessageResponse{
		Message:      MessageProto(stored),
		Deduplicated: deduplicated,
	}), nil
}

func (h *ChatHandler) SendDirectMessage(
	ctx context.Context,
	req *connect.Request[chatv1.SendDirectMessageRequest],
) (*connect.Response[chatv1.SendDirectMessageResponse], error) {
	current, err := caller(ctx)
	if err != nil {
		return nil, err
	}

	recipient, err := h.rules.NormalizeNick(req.Msg.GetRecipientNick())
	if err != nil {
		return nil, Error(err)
	}

	clientID := req.Msg.GetClientMessageId()
	if !chat.ValidID(clientID) {
		return nil, Error(chat.Fail(chat.ReasonClientMessageIDInvalid, "client_message_id"))
	}

	body, err := h.rules.NormalizeBody(req.Msg.GetBody())
	if err != nil {
		return nil, Error(err)
	}

	now := h.clock.Now()
	if rateErr := h.limiter.Allow(current.ID, now); rateErr != nil {
		return nil, Error(rateErr)
	}

	id, err := h.ids.New(now)
	if err != nil {
		return nil, Error(err)
	}

	message := chat.Message{
		ID:        id,
		ClientID:  clientID,
		Author:    current.Nick,
		Body:      body,
		Kind:      chat.KindDirect,
		CreatedAt: now,
	}

	target, lookupErr := h.store.SessionByNick(ctx, recipient)
	delivered := lookupErr == nil && h.hub.SessionOnline(target.ID)

	if delivered {
		eventID, eventErr := h.ids.New(now)
		if eventErr != nil {
			return nil, Error(eventErr)
		}
		event := chat.DirectMessageSent(eventID, now, message, recipient)
		h.hub.PublishSession(target.ID, event)
		if target.ID != current.ID {
			h.hub.PublishSession(current.ID, event)
		}
	}

	return connect.NewResponse(&chatv1.SendDirectMessageResponse{
		Message:   MessageProto(message),
		Delivered: delivered,
	}), nil
}

func (h *ChatHandler) SetTyping(
	ctx context.Context,
	req *connect.Request[chatv1.SetTypingRequest],
) (*connect.Response[chatv1.SetTypingResponse], error) {
	current, err := caller(ctx)
	if err != nil {
		return nil, err
	}

	channelID := req.Msg.GetChannelId()
	typing := req.Msg.GetTyping()

	now := h.clock.Now()
	until := time.Time{}
	if typing {
		until = now.Add(h.typing)
	}

	if err := h.store.SetTyping(ctx, channelID, current.ID, until); err != nil {
		return nil, Error(err)
	}

	if err := h.publishChannel(channelID, now, func(id string, at time.Time) chat.Event {
		return chat.TypingChanged(id, at, channelID, current.Nick, typing)
	}); err != nil {
		return nil, Error(err)
	}

	return connect.NewResponse(&chatv1.SetTypingResponse{}), nil
}

func (h *ChatHandler) StreamEvents(
	ctx context.Context,
	_ *connect.Request[chatv1.StreamEventsRequest],
	_ *connect.ServerStream[chatv1.StreamEventsResponse],
) error {
	if _, err := caller(ctx); err != nil {
		return err
	}
	return connect.NewError(connect.CodeUnimplemented, errStreamNotWired)
}

func (h *ChatHandler) seedCreationNotice(ctx context.Context, channel chat.Channel, now time.Time) error {
	id, err := h.ids.New(now)
	if err != nil {
		return err
	}

	notice := chat.Message{
		ID:        id,
		ChannelID: channel.ID,
		Author:    "*",
		Body:      channel.Name[1:] + CreatedChannelSuffix,
		Kind:      chat.KindSystem,
		CreatedAt: now,
	}
	stored, _, err := h.store.Append(ctx, notice)
	if err != nil {
		return err
	}

	return h.publishChannel(channel.ID, now, func(eventID string, at time.Time) chat.Event {
		return chat.MessageCreated(eventID, at, stored)
	})
}

func (h *ChatHandler) publishChannel(channelID string, now time.Time, build func(string, time.Time) chat.Event) error {
	id, err := h.ids.New(now)
	if err != nil {
		return err
	}
	h.hub.PublishChannel(channelID, build(id, now))
	return nil
}

func (h *ChatHandler) requireMembership(ctx context.Context, channelID, sessionID string) error {
	member, err := h.store.IsMember(ctx, channelID, sessionID)
	if err != nil {
		return err
	}
	if !member {
		return chat.Fail(chat.ReasonNotAMember, "channel_id")
	}
	return nil
}
