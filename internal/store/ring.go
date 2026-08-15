package store

import (
	"sort"
	"time"

	"github.com/reno/go-termchat/internal/chat"
)

type ring struct {
	buf       []chat.Message
	head      int
	n         int
	forgotten int
	byClient  map[string]string
}

func newRing(capacity int) *ring {
	return &ring{
		buf:      make([]chat.Message, capacity),
		byClient: make(map[string]string),
	}
}

func (r *ring) at(i int) chat.Message {
	return r.buf[(r.head+i)%len(r.buf)]
}

func (r *ring) len() int {
	return r.n
}

func (r *ring) lookup(clientID string) (chat.Message, bool) {
	if clientID == "" {
		return chat.Message{}, false
	}
	id, ok := r.byClient[clientID]
	if !ok {
		return chat.Message{}, false
	}
	i := r.indexOf(id)
	if i < 0 {
		delete(r.byClient, clientID)
		return chat.Message{}, false
	}
	return r.at(i), true
}

func (r *ring) push(m chat.Message) {
	if r.n == len(r.buf) {
		r.forget(r.at(0))
		r.buf[r.head] = m
		r.head = (r.head + 1) % len(r.buf)
	} else {
		r.buf[(r.head+r.n)%len(r.buf)] = m
		r.n++
	}
	if m.ClientID != "" {
		r.byClient[m.ClientID] = m.ID
	}
}

func (r *ring) dropBefore(cutoff time.Time) int {
	dropped := 0
	for r.n > 0 && r.at(0).CreatedAt.Before(cutoff) {
		r.forget(r.at(0))
		r.buf[r.head] = chat.Message{}
		r.head = (r.head + 1) % len(r.buf)
		r.n--
		dropped++
	}
	return dropped
}

func (r *ring) page(cursor string, limit int) Page {
	start := r.n - 1
	if cursor != "" {
		start = r.indexBefore(cursor)
	}

	msgs := make([]chat.Message, 0, limit)
	for i := start; i >= 0 && len(msgs) < limit; i-- {
		msgs = append(msgs, r.at(i))
	}

	exhausted := start-len(msgs) < 0
	page := Page{Messages: msgs, Truncated: exhausted && r.forgotten > 0}
	if !exhausted && len(msgs) > 0 {
		page.Next = msgs[len(msgs)-1].ID
	}
	return page
}

func (r *ring) forget(m chat.Message) {
	if m.ClientID != "" && r.byClient[m.ClientID] == m.ID {
		delete(r.byClient, m.ClientID)
	}
	r.forgotten++
}

func (r *ring) indexOf(id string) int {
	i := sort.Search(r.n, func(i int) bool { return r.at(i).ID >= id })
	if i < r.n && r.at(i).ID == id {
		return i
	}
	return -1
}

func (r *ring) indexBefore(id string) int {
	return sort.Search(r.n, func(i int) bool { return r.at(i).ID >= id }) - 1
}
