package store_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/reno/go-termchat/internal/chat"
	"github.com/reno/go-termchat/internal/store"
)

type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func newFakeClock(at time.Time) *fakeClock {
	return &fakeClock{now: at}
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

func TestSweepDropsMessagesPastTheirTTL(t *testing.T) {
	t.Parallel()

	f := newFixture(t, 10, 30*time.Minute)
	ch := f.channel(t, "?teste")

	f.send(t, ch.ID, "old", base)
	f.send(t, ch.ID, "recent", base.Add(25*time.Minute))

	stats, err := f.store.Sweep(f.ctx, base.Add(40*time.Minute))
	if err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if stats.MessagesDropped != 1 {
		t.Errorf("dropped %d messages, want 1", stats.MessagesDropped)
	}

	page, err := f.store.Messages(f.ctx, ch.ID, "", 10)
	if err != nil {
		t.Fatalf("Messages: %v", err)
	}
	if want := []string{"recent"}; !equal(bodies(page.Messages), want) {
		t.Errorf("messages = %v, want %v", bodies(page.Messages), want)
	}
	if !page.Truncated {
		t.Error("a TTL drop is still a forgotten message and must set Truncated")
	}
}

func TestSweepReapsAnEmptyChannel(t *testing.T) {
	t.Parallel()

	f := newFixture(t, 10, 30*time.Minute)
	ch := f.channel(t, "?teste")

	if err := f.store.CreateSession(f.ctx, chat.Session{ID: "s1", Nick: "rill", ExpiresAt: base.Add(time.Hour)}); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	f.join(t, ch.ID, "s1", "rill")
	f.send(t, ch.ID, "anyone here", base)

	if _, err := f.store.Sweep(f.ctx, base.Add(40*time.Minute)); err != nil {
		t.Fatalf("Sweep with a member present: %v", err)
	}
	if _, err := f.store.Channel(f.ctx, ch.ID); err != nil {
		t.Fatal("channel reaped while it still had a member")
	}

	if err := f.store.LeaveChannel(f.ctx, ch.ID, "s1"); err != nil {
		t.Fatalf("LeaveChannel: %v", err)
	}

	stats, err := f.store.Sweep(f.ctx, base.Add(80*time.Minute))
	if err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if stats.ChannelsReaped != 1 {
		t.Errorf("reaped %d channels, want 1", stats.ChannelsReaped)
	}

	if _, byID := f.store.Channel(f.ctx, ch.ID); chat.ReasonOf(byID) != chat.ReasonChannelNotFound {
		t.Errorf("channel still resolvable by id: %v", byID)
	}
	if _, byName := f.store.ChannelByName(f.ctx, "?teste"); chat.ReasonOf(byName) != chat.ReasonChannelNotFound {
		t.Errorf("channel still resolvable by name: %v", byName)
	}

	channels, err := f.store.Channels(f.ctx)
	if err != nil {
		t.Fatalf("Channels: %v", err)
	}
	if len(channels) != 0 {
		t.Errorf("store still holds %d channels, want 0", len(channels))
	}
}

func TestSweepExpiresSessionsAndReleasesTheirNicks(t *testing.T) {
	t.Parallel()

	f := newFixture(t, 10, time.Hour)
	ch := f.channel(t, "?teste")

	if err := f.store.CreateSession(f.ctx, chat.Session{ID: "s1", Nick: "rill", ExpiresAt: base.Add(10 * time.Minute)}); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	f.join(t, ch.ID, "s1", "rill")
	f.send(t, ch.ID, "still here", base.Add(15*time.Minute))

	stats, err := f.store.Sweep(f.ctx, base.Add(20*time.Minute))
	if err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if stats.SessionsExpired != 1 {
		t.Errorf("expired %d sessions, want 1", stats.SessionsExpired)
	}

	if _, lookup := f.store.Session(f.ctx, "s1"); chat.ReasonOf(lookup) != chat.ReasonSessionExpired {
		t.Errorf("expired session still resolvable: %v", lookup)
	}

	members, err := f.store.Members(f.ctx, ch.ID)
	if err != nil {
		t.Fatalf("Members: %v", err)
	}
	if len(members) != 0 {
		t.Errorf("expired session left %d members behind", len(members))
	}

	if err := f.store.CreateSession(f.ctx, chat.Session{ID: "s2", Nick: "rill", ExpiresAt: base.Add(time.Hour)}); err != nil {
		t.Errorf("nick not released on expiry: %v", err)
	}
}

func TestSweeperRunsOnItsTickerAndStopsWithTheContext(t *testing.T) {
	t.Parallel()

	f := newFixture(t, 10, 30*time.Minute)
	ch := f.channel(t, "?teste")
	f.send(t, ch.ID, "old", base)

	clock := newFakeClock(base.Add(time.Hour))
	swept := make(chan store.SweepStats, 1)

	sweeper := store.NewSweeper(f.store, time.Millisecond,
		store.WithClock(clock),
		store.WithSweepHook(func(s store.SweepStats) {
			select {
			case swept <- s:
			default:
			}
		}),
	)

	ctx, cancel := context.WithCancel(f.ctx)
	done := make(chan error, 1)
	go func() { done <- sweeper.Run(ctx) }()

	select {
	case stats := <-swept:
		if stats.MessagesDropped != 1 {
			t.Errorf("dropped %d messages, want 1", stats.MessagesDropped)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("sweeper never ran on its ticker")
	}

	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Run returned %v, want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("sweeper goroutine leaked past cancellation")
	}
}

func TestSweeperOnceUsesItsClock(t *testing.T) {
	t.Parallel()

	f := newFixture(t, 10, 30*time.Minute)
	ch := f.channel(t, "?teste")
	f.send(t, ch.ID, "old", base)

	clock := newFakeClock(base)
	sweeper := store.NewSweeper(f.store, time.Hour, store.WithClock(clock))

	stats, err := sweeper.Once(f.ctx)
	if err != nil {
		t.Fatalf("Once: %v", err)
	}
	if stats.MessagesDropped != 0 {
		t.Errorf("dropped %d messages before the TTL elapsed", stats.MessagesDropped)
	}

	clock.Advance(time.Hour)

	stats, err = sweeper.Once(f.ctx)
	if err != nil {
		t.Fatalf("Once: %v", err)
	}
	if stats.MessagesDropped != 1 {
		t.Errorf("dropped %d messages after the TTL elapsed, want 1", stats.MessagesDropped)
	}
}
