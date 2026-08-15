package chat

import "time"

type EventKind uint8

const (
	EventUnspecified EventKind = iota
	EventMessageCreated
	EventDirectMessage
	EventMemberJoined
	EventMemberLeft
	EventTypingChanged
	EventPresenceChanged
)

var eventKindNames = map[EventKind]string{
	EventUnspecified:     "unspecified",
	EventMessageCreated:  "message_created",
	EventDirectMessage:   "direct_message",
	EventMemberJoined:    "member_joined",
	EventMemberLeft:      "member_left",
	EventTypingChanged:   "typing_changed",
	EventPresenceChanged: "presence_changed",
}

func (k EventKind) String() string {
	if name, ok := eventKindNames[k]; ok {
		return name
	}
	return "unspecified"
}

type Event struct {
	ID        string
	EmittedAt time.Time
	Kind      EventKind

	ChannelID string
	Message   Message
	Recipient string
	Member    Member
	Nick      string
	Typing    bool
	Presence  Presence
}

func MessageCreated(id string, at time.Time, m Message) Event {
	return Event{
		ID:        id,
		EmittedAt: at,
		Kind:      EventMessageCreated,
		ChannelID: m.ChannelID,
		Message:   m,
	}
}

func DirectMessageSent(id string, at time.Time, m Message, recipient string) Event {
	return Event{
		ID:        id,
		EmittedAt: at,
		Kind:      EventDirectMessage,
		Message:   m,
		Recipient: recipient,
	}
}

func MemberJoined(id string, at time.Time, channelID string, m Member) Event {
	return Event{
		ID:        id,
		EmittedAt: at,
		Kind:      EventMemberJoined,
		ChannelID: channelID,
		Member:    m,
		Nick:      m.Nick,
	}
}

func MemberLeft(id string, at time.Time, channelID, nick string) Event {
	return Event{
		ID:        id,
		EmittedAt: at,
		Kind:      EventMemberLeft,
		ChannelID: channelID,
		Nick:      nick,
	}
}

func TypingChanged(id string, at time.Time, channelID, nick string, typing bool) Event {
	return Event{
		ID:        id,
		EmittedAt: at,
		Kind:      EventTypingChanged,
		ChannelID: channelID,
		Nick:      nick,
		Typing:    typing,
	}
}

func PresenceChanged(id string, at time.Time, nick string, p Presence) Event {
	return Event{
		ID:        id,
		EmittedAt: at,
		Kind:      EventPresenceChanged,
		Nick:      nick,
		Presence:  p,
	}
}
