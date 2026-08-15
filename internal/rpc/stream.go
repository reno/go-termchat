package rpc

import (
	"context"
	"errors"
	"sort"
	"time"

	"connectrpc.com/connect"

	chatv1 "github.com/reno/go-termchat/gen/go/chat/v1"
	"github.com/reno/go-termchat/internal/chat"
)

const ForgottenNotice = "[older messages were forgotten]"

var errSubscriberTooSlow = errors.New("event stream fell too far behind and was dropped")

func (h *ChatHandler) StreamEvents(
	ctx context.Context,
	req *connect.Request[chatv1.StreamEventsRequest],
	stream *connect.ServerStream[chatv1.StreamEventsResponse],
) error {
	current, err := caller(ctx)
	if err != nil {
		return err
	}

	channels, err := h.store.MemberChannels(ctx, current.ID)
	if err != nil {
		return Error(err)
	}

	sub := h.hub.Subscribe(current.ID, channels...)
	h.announcePresence(ctx, current, channels, chat.PresenceActive)

	defer h.announcePresence(context.WithoutCancel(ctx), current, channels, chat.PresenceIdle)
	defer h.hub.Unsubscribe(sub)

	replayed, err := h.replay(ctx, stream, channels, req.Msg.GetLastEventId())
	if err != nil {
		return Error(err)
	}

	keepalive := time.NewTicker(h.keepalive)
	defer keepalive.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil

		case event, open := <-sub.Events():
			if !open {
				return connect.NewError(connect.CodeResourceExhausted, errSubscriberTooSlow)
			}
			if replayed != "" && carriesMessage(event) && event.Message.ID <= replayed {
				continue
			}
			if err := stream.Send(&chatv1.StreamEventsResponse{Event: EventProto(event)}); err != nil {
				return err
			}

		case <-keepalive.C:
			if err := stream.Send(&chatv1.StreamEventsResponse{}); err != nil {
				return err
			}
		}
	}
}

func (h *ChatHandler) replay(
	ctx context.Context,
	stream *connect.ServerStream[chatv1.StreamEventsResponse],
	channels []string,
	lastEventID string,
) (string, error) {
	if lastEventID == "" {
		return "", nil
	}
	if !chat.ValidID(lastEventID) {
		return "", chat.Fail(chat.ReasonCursorInvalid, "last_event_id")
	}

	var (
		missed    []chat.Message
		truncated bool
	)
	for _, channelID := range channels {
		messages, gap, err := h.missedSince(ctx, channelID, lastEventID)
		if err != nil {
			return "", err
		}
		missed = append(missed, messages...)
		truncated = truncated || gap
	}

	sort.Slice(missed, func(i, j int) bool { return missed[i].ID < missed[j].ID })

	if truncated {
		if err := h.sendForgottenNotice(stream); err != nil {
			return "", err
		}
	}

	watermark := lastEventID
	for _, message := range missed {
		event := chat.MessageCreated(message.CreatedAt, message)
		if err := stream.Send(&chatv1.StreamEventsResponse{Event: EventProto(event)}); err != nil {
			return "", err
		}
		watermark = message.ID
	}
	return watermark, nil
}

func (h *ChatHandler) missedSince(ctx context.Context, channelID, lastEventID string) ([]chat.Message, bool, error) {
	var (
		missed []chat.Message
		cursor string
	)

	for {
		page, err := h.store.Messages(ctx, channelID, cursor, 0)
		if err != nil {
			return nil, false, err
		}

		for _, message := range page.Messages {
			if message.ID <= lastEventID {
				return missed, false, nil
			}
			missed = append(missed, message)
		}

		if page.Next == "" {
			return missed, page.Truncated, nil
		}
		cursor = page.Next
	}
}

func (h *ChatHandler) sendForgottenNotice(stream *connect.ServerStream[chatv1.StreamEventsResponse]) error {
	now := h.clock.Now()
	id, err := h.ids.New(now)
	if err != nil {
		return err
	}

	notice := chat.Message{
		ID:        id,
		Author:    "*",
		Body:      ForgottenNotice,
		Kind:      chat.KindSystem,
		CreatedAt: now,
	}
	return stream.Send(&chatv1.StreamEventsResponse{
		Event: EventProto(chat.MessageCreated(now, notice)),
	})
}

func (h *ChatHandler) announcePresence(
	ctx context.Context,
	current chat.Session,
	channels []string,
	presence chat.Presence,
) {
	if presence == chat.PresenceIdle && h.hub.SessionOnline(current.ID) {
		return
	}
	if err := h.store.SetPresence(ctx, current.ID, presence); err != nil {
		return
	}

	now := h.clock.Now()
	id, err := h.ids.New(now)
	if err != nil {
		return
	}

	event := chat.PresenceChanged(id, now, current.Nick, presence)
	for _, channelID := range channels {
		h.hub.PublishChannel(channelID, event)
	}
}

func carriesMessage(event chat.Event) bool {
	return event.Kind == chat.EventMessageCreated || event.Kind == chat.EventDirectMessage
}
