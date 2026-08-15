package hub

import (
	"sync"

	"github.com/reno/go-termchat/internal/chat"
)

const DefaultBuffer = 64

type Subscription struct {
	sessionID string
	events    chan chat.Event
	done      chan struct{}
	once      sync.Once
}

func (s *Subscription) SessionID() string {
	return s.sessionID
}

func (s *Subscription) Events() <-chan chat.Event {
	return s.events
}

func (s *Subscription) Done() <-chan struct{} {
	return s.done
}

func (s *Subscription) close() {
	s.once.Do(func() {
		close(s.done)
		close(s.events)
	})
}

type Hub struct {
	mu       sync.Mutex
	buffer   int
	sessions map[string]map[*Subscription]struct{}
	channels map[string]map[*Subscription]struct{}
	joined   map[*Subscription]map[string]struct{}
}

func New(buffer int) *Hub {
	if buffer <= 0 {
		buffer = DefaultBuffer
	}
	return &Hub{
		buffer:   buffer,
		sessions: make(map[string]map[*Subscription]struct{}),
		channels: make(map[string]map[*Subscription]struct{}),
		joined:   make(map[*Subscription]map[string]struct{}),
	}
}

func (h *Hub) Subscribe(sessionID string, channelIDs ...string) *Subscription {
	sub := &Subscription{
		sessionID: sessionID,
		events:    make(chan chat.Event, h.buffer),
		done:      make(chan struct{}),
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	if h.sessions[sessionID] == nil {
		h.sessions[sessionID] = make(map[*Subscription]struct{})
	}
	h.sessions[sessionID][sub] = struct{}{}
	h.joined[sub] = make(map[string]struct{}, len(channelIDs))

	for _, channelID := range channelIDs {
		h.attach(sub, channelID)
	}
	return sub
}

func (h *Hub) Unsubscribe(sub *Subscription) {
	if sub == nil {
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	h.detach(sub)
	sub.close()
}

func (h *Hub) JoinChannel(sessionID, channelID string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	for sub := range h.sessions[sessionID] {
		h.attach(sub, channelID)
	}
}

func (h *Hub) LeaveChannel(sessionID, channelID string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	for sub := range h.sessions[sessionID] {
		h.release(sub, channelID)
	}
}

func (h *Hub) PublishChannel(channelID string, event chat.Event) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.fanout(h.channels[channelID], event)
}

func (h *Hub) PublishSession(sessionID string, event chat.Event) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.fanout(h.sessions[sessionID], event)
}

func (h *Hub) Subscribers() int {
	h.mu.Lock()
	defer h.mu.Unlock()

	return len(h.joined)
}

func (h *Hub) SessionOnline(sessionID string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()

	return len(h.sessions[sessionID]) > 0
}

func (h *Hub) fanout(targets map[*Subscription]struct{}, event chat.Event) {
	var slow []*Subscription

	for sub := range targets {
		select {
		case sub.events <- event:
		default:
			slow = append(slow, sub)
		}
	}

	for _, sub := range slow {
		h.detach(sub)
		sub.close()
	}
}

func (h *Hub) attach(sub *Subscription, channelID string) {
	if _, ok := h.joined[sub]; !ok {
		return
	}
	if h.channels[channelID] == nil {
		h.channels[channelID] = make(map[*Subscription]struct{})
	}
	h.channels[channelID][sub] = struct{}{}
	h.joined[sub][channelID] = struct{}{}
}

func (h *Hub) release(sub *Subscription, channelID string) {
	delete(h.channels[channelID], sub)
	if len(h.channels[channelID]) == 0 {
		delete(h.channels, channelID)
	}
	if members, ok := h.joined[sub]; ok {
		delete(members, channelID)
	}
}

func (h *Hub) detach(sub *Subscription) {
	for channelID := range h.joined[sub] {
		delete(h.channels[channelID], sub)
		if len(h.channels[channelID]) == 0 {
			delete(h.channels, channelID)
		}
	}
	delete(h.joined, sub)

	delete(h.sessions[sub.sessionID], sub)
	if len(h.sessions[sub.sessionID]) == 0 {
		delete(h.sessions, sub.sessionID)
	}
}
