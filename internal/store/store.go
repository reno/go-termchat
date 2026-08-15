package store

import (
	"context"
	"time"

	"github.com/reno/go-termchat/internal/chat"
)

type Page struct {
	Messages  []chat.Message
	Next      string
	Truncated bool
}

type SweepStats struct {
	MessagesDropped int
	ChannelsReaped  int
	SessionsExpired int
}

type Clock interface {
	Now() time.Time
}

type SystemClock struct{}

func (SystemClock) Now() time.Time { return time.Now() }

type Store interface {
	CreateSession(ctx context.Context, s chat.Session) error
	Session(ctx context.Context, id string) (chat.Session, error)
	TouchSession(ctx context.Context, id string, expiresAt time.Time) (chat.Session, error)
	DeleteSession(ctx context.Context, id string) error
	SessionByNick(ctx context.Context, nick string) (chat.Session, error)
	SetNick(ctx context.Context, id, nick string) (chat.Session, error)

	EnsureChannel(ctx context.Context, id, name string, at time.Time) (chat.Channel, bool, error)
	Channel(ctx context.Context, id string) (chat.Channel, error)
	ChannelByName(ctx context.Context, name string) (chat.Channel, error)
	Channels(ctx context.Context) ([]chat.Channel, error)

	JoinChannel(ctx context.Context, channelID string, m chat.Member) error
	LeaveChannel(ctx context.Context, channelID, sessionID string) error
	LeaveAll(ctx context.Context, sessionID string) ([]string, error)
	Members(ctx context.Context, channelID string) ([]chat.Member, error)
	IsMember(ctx context.Context, channelID, sessionID string) (bool, error)
	MemberChannels(ctx context.Context, sessionID string) ([]string, error)
	SetTyping(ctx context.Context, channelID, sessionID string, until time.Time) error
	SetPresence(ctx context.Context, sessionID string, p chat.Presence) error

	Append(ctx context.Context, m chat.Message) (chat.Message, bool, error)
	Messages(ctx context.Context, channelID, cursor string, limit int) (Page, error)

	Sweep(ctx context.Context, now time.Time) (SweepStats, error)
}
