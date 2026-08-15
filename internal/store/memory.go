package store

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/reno/go-termchat/internal/chat"
	"github.com/reno/go-termchat/internal/config"
)

const defaultPageSize = 50

type channelState struct {
	channel chat.Channel
	members map[string]chat.Member
	msgs    *ring
}

type Memory struct {
	mu sync.RWMutex

	retention config.Retention
	maxPage   int

	sessions map[string]chat.Session
	nicks    map[string]string
	channels map[string]*channelState
	byName   map[string]string
	memberOf map[string]map[string]struct{}
}

var _ Store = (*Memory)(nil)

func NewMemory(retention config.Retention) *Memory {
	return &Memory{
		retention: retention,
		maxPage:   retention.MessagesPerChannel,
		sessions:  make(map[string]chat.Session),
		nicks:     make(map[string]string),
		channels:  make(map[string]*channelState),
		byName:    make(map[string]string),
		memberOf:  make(map[string]map[string]struct{}),
	}
}

func (m *Memory) CreateSession(_ context.Context, s chat.Session) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	folded := chat.FoldNick(s.Nick)
	if holder, taken := m.nicks[folded]; taken && holder != s.ID {
		return chat.Fail(chat.ReasonNickTaken, "nick")
	}
	m.sessions[s.ID] = s
	m.nicks[folded] = s.ID
	return nil
}

func (m *Memory) Session(_ context.Context, id string) (chat.Session, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	s, ok := m.sessions[id]
	if !ok {
		return chat.Session{}, chat.Fail(chat.ReasonSessionExpired, "session")
	}
	return s, nil
}

func (m *Memory) TouchSession(_ context.Context, id string, expiresAt time.Time) (chat.Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	s, ok := m.sessions[id]
	if !ok {
		return chat.Session{}, chat.Fail(chat.ReasonSessionExpired, "session")
	}
	s.ExpiresAt = expiresAt
	m.sessions[id] = s
	return s, nil
}

func (m *Memory) DeleteSession(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.dropSession(id)
	return nil
}

func (m *Memory) SessionByNick(_ context.Context, nick string) (chat.Session, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	id, ok := m.nicks[chat.FoldNick(nick)]
	if !ok {
		return chat.Session{}, chat.Fail(chat.ReasonRecipientOffline, "nick")
	}
	s, ok := m.sessions[id]
	if !ok {
		return chat.Session{}, chat.Fail(chat.ReasonRecipientOffline, "nick")
	}
	return s, nil
}

func (m *Memory) SetNick(_ context.Context, id, nick string) (chat.Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	s, ok := m.sessions[id]
	if !ok {
		return chat.Session{}, chat.Fail(chat.ReasonSessionExpired, "session")
	}

	folded := chat.FoldNick(nick)
	if holder, taken := m.nicks[folded]; taken && holder != id {
		return chat.Session{}, chat.Fail(chat.ReasonNickTaken, "nick")
	}

	delete(m.nicks, chat.FoldNick(s.Nick))
	m.nicks[folded] = id
	s.Nick = nick
	m.sessions[id] = s

	for channelID := range m.memberOf[id] {
		state, ok := m.channels[channelID]
		if !ok {
			continue
		}
		member, ok := state.members[id]
		if !ok {
			continue
		}
		member.Nick = nick
		state.members[id] = member
	}
	return s, nil
}

func (m *Memory) EnsureChannel(_ context.Context, id, name string, at time.Time) (chat.Channel, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if existing, ok := m.byName[name]; ok {
		return m.channels[existing].channel, false, nil
	}

	state := &channelState{
		channel: chat.Channel{ID: id, Name: name, CreatedAt: at},
		members: make(map[string]chat.Member),
		msgs:    newRing(m.retention.MessagesPerChannel),
	}
	m.channels[id] = state
	m.byName[name] = id
	return state.channel, true, nil
}

func (m *Memory) Channel(_ context.Context, id string) (chat.Channel, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	state, ok := m.channels[id]
	if !ok {
		return chat.Channel{}, chat.Fail(chat.ReasonChannelNotFound, "channel_id")
	}
	return state.channel, nil
}

func (m *Memory) ChannelByName(_ context.Context, name string) (chat.Channel, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	id, ok := m.byName[name]
	if !ok {
		return chat.Channel{}, chat.Fail(chat.ReasonChannelNotFound, "name")
	}
	return m.channels[id].channel, nil
}

func (m *Memory) Channels(_ context.Context) ([]chat.Channel, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	out := make([]chat.Channel, 0, len(m.channels))
	for _, state := range m.channels {
		out = append(out, state.channel)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func (m *Memory) JoinChannel(_ context.Context, channelID string, member chat.Member) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	state, ok := m.channels[channelID]
	if !ok {
		return chat.Fail(chat.ReasonChannelNotFound, "channel_id")
	}
	state.members[member.SessionID] = member
	if m.memberOf[member.SessionID] == nil {
		m.memberOf[member.SessionID] = make(map[string]struct{})
	}
	m.memberOf[member.SessionID][channelID] = struct{}{}
	return nil
}

func (m *Memory) LeaveChannel(_ context.Context, channelID, sessionID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	state, ok := m.channels[channelID]
	if !ok {
		return chat.Fail(chat.ReasonChannelNotFound, "channel_id")
	}
	delete(state.members, sessionID)
	delete(m.memberOf[sessionID], channelID)
	if len(m.memberOf[sessionID]) == 0 {
		delete(m.memberOf, sessionID)
	}
	return nil
}

func (m *Memory) LeaveAll(_ context.Context, sessionID string) ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.leaveAll(sessionID), nil
}

func (m *Memory) Members(_ context.Context, channelID string) ([]chat.Member, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	state, ok := m.channels[channelID]
	if !ok {
		return nil, chat.Fail(chat.ReasonChannelNotFound, "channel_id")
	}
	out := make([]chat.Member, 0, len(state.members))
	for _, member := range state.members {
		out = append(out, member)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].JoinedAt.Equal(out[j].JoinedAt) {
			return out[i].Nick < out[j].Nick
		}
		return out[i].JoinedAt.Before(out[j].JoinedAt)
	})
	return out, nil
}

func (m *Memory) IsMember(_ context.Context, channelID, sessionID string) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	state, ok := m.channels[channelID]
	if !ok {
		return false, chat.Fail(chat.ReasonChannelNotFound, "channel_id")
	}
	_, member := state.members[sessionID]
	return member, nil
}

func (m *Memory) MemberChannels(_ context.Context, sessionID string) ([]string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	out := make([]string, 0, len(m.memberOf[sessionID]))
	for channelID := range m.memberOf[sessionID] {
		out = append(out, channelID)
	}
	sort.Strings(out)
	return out, nil
}

func (m *Memory) SetTyping(_ context.Context, channelID, sessionID string, until time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	state, ok := m.channels[channelID]
	if !ok {
		return chat.Fail(chat.ReasonChannelNotFound, "channel_id")
	}
	member, ok := state.members[sessionID]
	if !ok {
		return chat.Fail(chat.ReasonNotAMember, "channel_id")
	}
	member.TypingUntil = until
	state.members[sessionID] = member
	return nil
}

func (m *Memory) SetPresence(_ context.Context, sessionID string, p chat.Presence) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for channelID := range m.memberOf[sessionID] {
		state, ok := m.channels[channelID]
		if !ok {
			continue
		}
		member, ok := state.members[sessionID]
		if !ok {
			continue
		}
		member.Presence = p
		state.members[sessionID] = member
	}
	return nil
}

func (m *Memory) Append(_ context.Context, msg chat.Message) (chat.Message, bool, error) {
	if msg.Kind == chat.KindDirect {
		return chat.Message{}, false, chat.Fail(chat.ReasonMessageKindInvalid, "kind")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	state, ok := m.channels[msg.ChannelID]
	if !ok {
		return chat.Message{}, false, chat.Fail(chat.ReasonChannelNotFound, "channel_id")
	}
	if existing, found := state.msgs.lookup(msg.ClientID); found {
		return existing, true, nil
	}
	state.msgs.push(msg)
	return msg, false, nil
}

func (m *Memory) Messages(_ context.Context, channelID, cursor string, limit int) (Page, error) {
	if cursor != "" && !chat.ValidID(cursor) {
		return Page{}, chat.Fail(chat.ReasonCursorInvalid, "cursor")
	}
	if limit <= 0 {
		limit = defaultPageSize
	}
	if limit > m.maxPage {
		limit = m.maxPage
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	state, ok := m.channels[channelID]
	if !ok {
		return Page{}, chat.Fail(chat.ReasonChannelNotFound, "channel_id")
	}
	return state.msgs.page(cursor, limit), nil
}

func (m *Memory) Sweep(_ context.Context, now time.Time) (SweepStats, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var stats SweepStats

	for id, s := range m.sessions {
		if s.Expired(now) {
			m.dropSession(id)
			stats.SessionsExpired++
		}
	}

	cutoff := now.Add(-m.retention.MessageTTL)
	for id, state := range m.channels {
		stats.MessagesDropped += state.msgs.dropBefore(cutoff)
		if len(state.members) == 0 && state.msgs.len() == 0 {
			delete(m.channels, id)
			delete(m.byName, state.channel.Name)
			stats.ChannelsReaped++
		}
	}
	return stats, nil
}

func (m *Memory) dropSession(id string) {
	s, ok := m.sessions[id]
	if !ok {
		return
	}
	m.leaveAll(id)
	delete(m.sessions, id)
	if m.nicks[chat.FoldNick(s.Nick)] == id {
		delete(m.nicks, chat.FoldNick(s.Nick))
	}
}

func (m *Memory) leaveAll(sessionID string) []string {
	left := make([]string, 0, len(m.memberOf[sessionID]))
	for channelID := range m.memberOf[sessionID] {
		if state, ok := m.channels[channelID]; ok {
			delete(state.members, sessionID)
		}
		left = append(left, channelID)
	}
	delete(m.memberOf, sessionID)
	sort.Strings(left)
	return left
}
