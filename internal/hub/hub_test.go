package hub_test

import (
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/reno/go-termchat/internal/chat"
	"github.com/reno/go-termchat/internal/hub"
)

var at = time.Date(2026, time.August, 15, 12, 0, 0, 0, time.UTC)

func event(id string) chat.Event {
	return chat.MessageCreated(id, at, chat.Message{
		ID:        id,
		ChannelID: "c1",
		Author:    "rill",
		Body:      "hello",
		Kind:      chat.KindNormal,
		CreatedAt: at,
	})
}

func receive(t *testing.T, sub *hub.Subscription) chat.Event {
	t.Helper()

	select {
	case got := <-sub.Events():
		return got
	case <-time.After(2 * time.Second):
		t.Fatal("no event arrived")
		return chat.Event{}
	}
}

func expectQuiet(t *testing.T, sub *hub.Subscription) {
	t.Helper()

	select {
	case got := <-sub.Events():
		t.Fatalf("unexpected event %s", got.ID)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestChannelPublishReachesEveryMember(t *testing.T) {
	t.Parallel()

	h := hub.New(4)
	first := h.Subscribe("s1", "c1")
	second := h.Subscribe("s2", "c1")
	outsider := h.Subscribe("s3", "c2")

	h.PublishChannel("c1", event("e1"))

	if got := receive(t, first); got.ID != "e1" {
		t.Errorf("first got %s", got.ID)
	}
	if got := receive(t, second); got.ID != "e1" {
		t.Errorf("second got %s", got.ID)
	}
	expectQuiet(t, outsider)
}

func TestSessionPublishReachesOnlyThatSession(t *testing.T) {
	t.Parallel()

	h := hub.New(4)
	target := h.Subscribe("s1", "c1")
	other := h.Subscribe("s2", "c1")

	h.PublishSession("s1", event("dm"))

	if got := receive(t, target); got.ID != "dm" {
		t.Errorf("target got %s", got.ID)
	}
	expectQuiet(t, other)
}

func TestJoinAndLeaveChannelMoveExistingSubscriptions(t *testing.T) {
	t.Parallel()

	h := hub.New(4)
	sub := h.Subscribe("s1")

	h.PublishChannel("c1", event("before"))
	expectQuiet(t, sub)

	h.JoinChannel("s1", "c1")
	h.PublishChannel("c1", event("after-join"))
	if got := receive(t, sub); got.ID != "after-join" {
		t.Errorf("got %s", got.ID)
	}

	h.LeaveChannel("s1", "c1")
	h.PublishChannel("c1", event("after-leave"))
	expectQuiet(t, sub)
}

func TestUnsubscribeClosesTheChannelAndStopsDelivery(t *testing.T) {
	t.Parallel()

	h := hub.New(4)
	sub := h.Subscribe("s1", "c1")

	h.Unsubscribe(sub)

	select {
	case <-sub.Done():
	case <-time.After(time.Second):
		t.Fatal("Done was not closed")
	}

	h.PublishChannel("c1", event("e1"))

	if _, open := <-sub.Events(); open {
		t.Error("events channel still delivering after Unsubscribe")
	}
	if h.Subscribers() != 0 {
		t.Errorf("Subscribers = %d, want 0", h.Subscribers())
	}
}

func TestUnsubscribeIsIdempotent(t *testing.T) {
	t.Parallel()

	h := hub.New(4)
	sub := h.Subscribe("s1", "c1")

	h.Unsubscribe(sub)
	h.Unsubscribe(sub)
	h.Unsubscribe(nil)
}

func TestASlowSubscriberIsDroppedRatherThanBlockingThePublisher(t *testing.T) {
	t.Parallel()

	h := hub.New(2)
	slow := h.Subscribe("slow", "c1")
	healthy := h.Subscribe("healthy", "c1")

	for i := range 10 {
		h.PublishChannel("c1", event("e"+strconv.Itoa(i)))
		select {
		case <-healthy.Events():
		default:
		}
	}

	select {
	case <-slow.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("the slow subscriber was never dropped, the publisher would have blocked")
	}

	if h.SessionOnline("slow") {
		t.Error("dropped subscriber still counts as online")
	}
	if !h.SessionOnline("healthy") {
		t.Error("a subscriber that kept up was dropped alongside the slow one")
	}

	h.Unsubscribe(healthy)
}

func TestOneHundredConcurrentSubscribersAllReceive(t *testing.T) {
	t.Parallel()

	const subscribers = 100

	h := hub.New(8)

	var (
		wg     sync.WaitGroup
		mu     sync.Mutex
		counts = make(map[string]int, subscribers)
		subs   = make([]*hub.Subscription, 0, subscribers)
	)

	for i := range subscribers {
		sessionID := "s" + strconv.Itoa(i)
		sub := h.Subscribe(sessionID, "c1")
		subs = append(subs, sub)

		wg.Add(1)
		go func() {
			defer wg.Done()
			for range sub.Events() {
				mu.Lock()
				counts[sessionID]++
				mu.Unlock()
			}
		}()
	}

	if h.Subscribers() != subscribers {
		t.Fatalf("Subscribers = %d, want %d", h.Subscribers(), subscribers)
	}

	const published = 5
	for i := range published {
		h.PublishChannel("c1", event("e"+strconv.Itoa(i)))
	}

	deadline := time.After(5 * time.Second)
	for {
		mu.Lock()
		complete := len(counts) == subscribers
		for _, n := range counts {
			if n < published {
				complete = false
				break
			}
		}
		mu.Unlock()

		if complete {
			break
		}
		select {
		case <-deadline:
			mu.Lock()
			t.Fatalf("only %d of %d subscribers received every event", len(counts), subscribers)
			mu.Unlock()
		case <-time.After(10 * time.Millisecond):
		}
	}

	for _, sub := range subs {
		h.Unsubscribe(sub)
	}
	wg.Wait()

	if h.Subscribers() != 0 {
		t.Errorf("Subscribers = %d after teardown, want 0", h.Subscribers())
	}
}

func TestConcurrentSubscribeAndPublishAreRaceFree(t *testing.T) {
	t.Parallel()

	h := hub.New(16)

	var wg sync.WaitGroup
	stop := make(chan struct{})

	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
				h.PublishChannel("c1", event("e"))
				h.PublishSession("s0", event("d"))
			}
		}
	}()

	for i := range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sessionID := "s" + strconv.Itoa(i)
			for range 20 {
				sub := h.Subscribe(sessionID, "c1")
				h.JoinChannel(sessionID, "c2")
				h.LeaveChannel(sessionID, "c1")
				h.Unsubscribe(sub)
			}
		}()
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		time.Sleep(100 * time.Millisecond)
		close(stop)
	}()

	wg.Wait()

	if h.Subscribers() != 0 {
		t.Errorf("Subscribers = %d after teardown, want 0", h.Subscribers())
	}
}
