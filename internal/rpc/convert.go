package rpc

import (
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	chatv1 "github.com/reno/go-termchat/gen/go/chat/v1"
	"github.com/reno/go-termchat/internal/chat"
)

var (
	kindsToProto = map[chat.MessageKind]chatv1.MessageKind{
		chat.KindUnspecified: chatv1.MessageKind_MESSAGE_KIND_UNSPECIFIED,
		chat.KindNormal:      chatv1.MessageKind_MESSAGE_KIND_NORMAL,
		chat.KindSystem:      chatv1.MessageKind_MESSAGE_KIND_SYSTEM,
		chat.KindAction:      chatv1.MessageKind_MESSAGE_KIND_ACTION,
		chat.KindDirect:      chatv1.MessageKind_MESSAGE_KIND_DIRECT,
	}
	kindsFromProto = map[chatv1.MessageKind]chat.MessageKind{
		chatv1.MessageKind_MESSAGE_KIND_UNSPECIFIED: chat.KindUnspecified,
		chatv1.MessageKind_MESSAGE_KIND_NORMAL:      chat.KindNormal,
		chatv1.MessageKind_MESSAGE_KIND_SYSTEM:      chat.KindSystem,
		chatv1.MessageKind_MESSAGE_KIND_ACTION:      chat.KindAction,
		chatv1.MessageKind_MESSAGE_KIND_DIRECT:      chat.KindDirect,
	}
	presenceToProto = map[chat.Presence]chatv1.Presence{
		chat.PresenceUnspecified: chatv1.Presence_PRESENCE_UNSPECIFIED,
		chat.PresenceActive:      chatv1.Presence_PRESENCE_ACTIVE,
		chat.PresenceIdle:        chatv1.Presence_PRESENCE_IDLE,
	}
)

func Kind(k chat.MessageKind) chatv1.MessageKind {
	return kindsToProto[k]
}

func KindFrom(k chatv1.MessageKind) chat.MessageKind {
	return kindsFromProto[k]
}

func SessionProto(s chat.Session) *chatv1.Session {
	return &chatv1.Session{
		Nick:      s.Nick,
		CreatedAt: timestamppb.New(s.CreatedAt),
		ExpiresAt: timestamppb.New(s.ExpiresAt),
	}
}

func MemberProto(m chat.Member, now time.Time) *chatv1.Member {
	return &chatv1.Member{
		Nick:     m.Nick,
		Presence: presenceToProto[m.Presence],
		Typing:   m.Typing(now),
		JoinedAt: timestamppb.New(m.JoinedAt),
	}
}

func MessageProto(m chat.Message) *chatv1.Message {
	return &chatv1.Message{
		Id:              m.ID,
		ClientMessageId: m.ClientID,
		ChannelId:       m.ChannelID,
		AuthorNick:      m.Author,
		Body:            m.Body,
		Kind:            Kind(m.Kind),
		CreatedAt:       timestamppb.New(m.CreatedAt),
	}
}

func EventProto(e chat.Event) *chatv1.Event {
	event := &chatv1.Event{
		EventId:   e.ID,
		EmittedAt: timestamppb.New(e.EmittedAt),
	}

	switch e.Kind {
	case chat.EventMessageCreated:
		event.Payload = &chatv1.Event_MessageCreated{
			MessageCreated: &chatv1.MessageCreated{Message: MessageProto(e.Message)},
		}
	case chat.EventDirectMessage:
		event.Payload = &chatv1.Event_DirectMessage{
			DirectMessage: &chatv1.DirectMessage{
				Message:       MessageProto(e.Message),
				RecipientNick: e.Recipient,
			},
		}
	case chat.EventMemberJoined:
		event.Payload = &chatv1.Event_MemberJoined{
			MemberJoined: &chatv1.MemberJoined{
				ChannelId: e.ChannelID,
				Member:    MemberProto(e.Member, e.EmittedAt),
			},
		}
	case chat.EventMemberLeft:
		event.Payload = &chatv1.Event_MemberLeft{
			MemberLeft: &chatv1.MemberLeft{ChannelId: e.ChannelID, Nick: e.Nick},
		}
	case chat.EventTypingChanged:
		event.Payload = &chatv1.Event_TypingChanged{
			TypingChanged: &chatv1.TypingChanged{
				ChannelId: e.ChannelID,
				Nick:      e.Nick,
				Typing:    e.Typing,
			},
		}
	case chat.EventPresenceChanged:
		event.Payload = &chatv1.Event_PresenceChanged{
			PresenceChanged: &chatv1.PresenceChanged{
				Nick:     e.Nick,
				Presence: presenceToProto[e.Presence],
			},
		}
	case chat.EventUnspecified:
	}

	return event
}

func ChannelProto(c chat.Channel, members int) *chatv1.Channel {
	return &chatv1.Channel{
		Id:          c.ID,
		Name:        c.Name,
		MemberCount: uint32(members), //nolint:gosec // member counts are bounded by connected sessions
	}
}
