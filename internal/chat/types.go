package chat

import "time"

type MessageKind uint8

const (
	KindUnspecified MessageKind = iota
	KindNormal
	KindSystem
	KindAction
	KindDirect
)

var messageKindNames = map[MessageKind]string{
	KindUnspecified: "unspecified",
	KindNormal:      "normal",
	KindSystem:      "system",
	KindAction:      "action",
	KindDirect:      "direct",
}

func (k MessageKind) String() string {
	if name, ok := messageKindNames[k]; ok {
		return name
	}
	return "unspecified"
}

func (k MessageKind) Valid() bool {
	_, ok := messageKindNames[k]
	return ok && k != KindUnspecified
}

type Presence uint8

const (
	PresenceUnspecified Presence = iota
	PresenceActive
	PresenceIdle
)

var presenceNames = map[Presence]string{
	PresenceUnspecified: "unspecified",
	PresenceActive:      "active",
	PresenceIdle:        "idle",
}

func (p Presence) String() string {
	if name, ok := presenceNames[p]; ok {
		return name
	}
	return "unspecified"
}

type Message struct {
	ID        string
	ClientID  string
	ChannelID string
	Author    string
	Body      string
	Kind      MessageKind
	CreatedAt time.Time
}

type Channel struct {
	ID        string
	Name      string
	CreatedAt time.Time
}

type Member struct {
	SessionID   string
	Nick        string
	JoinedAt    time.Time
	Presence    Presence
	TypingUntil time.Time
}

func (m Member) Typing(now time.Time) bool {
	return now.Before(m.TypingUntil)
}

type Session struct {
	ID        string
	Nick      string
	CreatedAt time.Time
	ExpiresAt time.Time
}

func (s Session) Expired(now time.Time) bool {
	return !now.Before(s.ExpiresAt)
}
