package session

import (
	"context"
	"time"

	"github.com/reno/go-termchat/internal/chat"
	"github.com/reno/go-termchat/internal/store"
)

type Manager struct {
	store   store.Store
	signer  *Signer
	rules   chat.Rules
	cookies Cookies
	clock   store.Clock
	ttl     time.Duration
}

type Option func(*Manager)

func WithClock(c store.Clock) Option {
	return func(m *Manager) { m.clock = c }
}

func NewManager(st store.Store, signer *Signer, rules chat.Rules, cookies Cookies, ttl time.Duration, opts ...Option) *Manager {
	m := &Manager{
		store:   st,
		signer:  signer,
		rules:   rules,
		cookies: cookies,
		clock:   store.SystemClock{},
		ttl:     ttl,
	}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

func (m *Manager) Cookies() Cookies {
	return m.cookies
}

func (m *Manager) TTL() time.Duration {
	return m.ttl
}

func (m *Manager) Join(ctx context.Context, nick string) (chat.Session, string, error) {
	normalized, err := m.rules.NormalizeNick(nick)
	if err != nil {
		return chat.Session{}, "", err
	}

	id, token, err := m.signer.Mint()
	if err != nil {
		return chat.Session{}, "", err
	}

	now := m.clock.Now()
	s := chat.Session{
		ID:        id,
		Nick:      normalized,
		CreatedAt: now,
		ExpiresAt: now.Add(m.ttl),
	}
	if err := m.store.CreateSession(ctx, s); err != nil {
		return chat.Session{}, "", err
	}
	return s, token, nil
}

func (m *Manager) Resolve(ctx context.Context, token string) (chat.Session, error) {
	if token == "" {
		return chat.Session{}, chat.Fail(chat.ReasonSessionExpired, "session")
	}

	id, err := m.signer.Verify(token)
	if err != nil {
		return chat.Session{}, chat.Fail(chat.ReasonSessionExpired, "session")
	}

	s, err := m.store.Session(ctx, id)
	if err != nil {
		return chat.Session{}, err
	}

	now := m.clock.Now()
	if s.Expired(now) {
		if err := m.store.DeleteSession(ctx, id); err != nil {
			return chat.Session{}, err
		}
		return chat.Session{}, chat.Fail(chat.ReasonSessionExpired, "session")
	}

	return m.store.TouchSession(ctx, id, now.Add(m.ttl))
}

func (m *Manager) SetNick(ctx context.Context, sessionID, nick string) (chat.Session, error) {
	normalized, err := m.rules.NormalizeNick(nick)
	if err != nil {
		return chat.Session{}, err
	}
	return m.store.SetNick(ctx, sessionID, normalized)
}

func (m *Manager) Leave(ctx context.Context, sessionID string) error {
	return m.store.DeleteSession(ctx, sessionID)
}

func (m *Manager) Expiry() time.Time {
	return m.clock.Now().Add(m.ttl)
}
