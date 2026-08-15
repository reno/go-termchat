package store_test

import (
	"context"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/reno/go-termchat/internal/chat"
	"github.com/reno/go-termchat/internal/config"
	"github.com/reno/go-termchat/internal/store"
)

var base = time.Date(2026, time.August, 14, 12, 0, 0, 0, time.UTC)

type fixture struct {
	store *store.Memory
	gen   *chat.IDGen
	ctx   context.Context
}

func newFixture(t *testing.T, capacity int, ttl time.Duration) fixture {
	t.Helper()

	return fixture{
		store: store.NewMemory(config.Retention{
			MessagesPerChannel: capacity,
			MessageTTL:         ttl,
			SweepInterval:      time.Minute,
		}),
		gen: chat.NewIDGen(),
		ctx: t.Context(),
	}
}

func (f fixture) id(t *testing.T, at time.Time) string {
	t.Helper()

	id, err := f.gen.New(at)
	if err != nil {
		t.Fatalf("id: %v", err)
	}
	return id
}

func (f fixture) channel(t *testing.T, name string) chat.Channel {
	t.Helper()

	ch, _, err := f.store.EnsureChannel(f.ctx, f.id(t, base), name, base)
	if err != nil {
		t.Fatalf("EnsureChannel: %v", err)
	}
	return ch
}

func (f fixture) send(t *testing.T, channelID, body string, at time.Time) chat.Message {
	t.Helper()

	msg := chat.Message{
		ID:        f.id(t, at),
		ClientID:  "client-" + body,
		ChannelID: channelID,
		Author:    "rill",
		Body:      body,
		Kind:      chat.KindNormal,
		CreatedAt: at,
	}
	stored, _, err := f.store.Append(f.ctx, msg)
	if err != nil {
		t.Fatalf("Append(%q): %v", body, err)
	}
	return stored
}

func (f fixture) join(t *testing.T, channelID, sessionID, nick string) {
	t.Helper()

	member := chat.Member{
		SessionID: sessionID,
		Nick:      nick,
		JoinedAt:  base,
		Presence:  chat.PresenceActive,
	}
	if err := f.store.JoinChannel(f.ctx, channelID, member); err != nil {
		t.Fatalf("JoinChannel: %v", err)
	}
}

func bodies(msgs []chat.Message) []string {
	out := make([]string, len(msgs))
	for i, m := range msgs {
		out[i] = m.Body
	}
	return out
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestAppendEvictsOldestBeyondCapacity(t *testing.T) {
	t.Parallel()

	f := newFixture(t, 3, time.Hour)
	ch := f.channel(t, "?teste")

	for i := range 4 {
		f.send(t, ch.ID, strconv.Itoa(i), base.Add(time.Duration(i)*time.Second))
	}

	page, err := f.store.Messages(f.ctx, ch.ID, "", 10)
	if err != nil {
		t.Fatalf("Messages: %v", err)
	}
	if want := []string{"3", "2", "1"}; !equal(bodies(page.Messages), want) {
		t.Errorf("messages = %v, want %v", bodies(page.Messages), want)
	}
	if !page.Truncated {
		t.Error("expected Truncated after an eviction")
	}
}

func TestMessagesPaginateAcrossTheEvictionBoundary(t *testing.T) {
	t.Parallel()

	f := newFixture(t, 4, time.Hour)
	ch := f.channel(t, "?teste")

	for i := range 6 {
		f.send(t, ch.ID, strconv.Itoa(i), base.Add(time.Duration(i)*time.Second))
	}

	first, err := f.store.Messages(f.ctx, ch.ID, "", 2)
	if err != nil {
		t.Fatalf("first page: %v", err)
	}
	if want := []string{"5", "4"}; !equal(bodies(first.Messages), want) {
		t.Fatalf("first page = %v, want %v", bodies(first.Messages), want)
	}
	if first.Truncated {
		t.Error("first page should not be truncated, older messages survive")
	}
	if first.Next == "" {
		t.Fatal("first page should carry a cursor")
	}

	second, err := f.store.Messages(f.ctx, ch.ID, first.Next, 2)
	if err != nil {
		t.Fatalf("second page: %v", err)
	}
	if want := []string{"3", "2"}; !equal(bodies(second.Messages), want) {
		t.Fatalf("second page = %v, want %v", bodies(second.Messages), want)
	}
	if !second.Truncated {
		t.Error("second page reaches the eviction boundary and must say so")
	}
	if second.Next != "" {
		t.Errorf("second page should not carry a cursor, got %q", second.Next)
	}
}

func TestMessagesWithCursorOlderThanTheWindow(t *testing.T) {
	t.Parallel()

	f := newFixture(t, 2, time.Hour)
	ch := f.channel(t, "?teste")

	oldest := f.send(t, ch.ID, "0", base)
	for i := 1; i < 4; i++ {
		f.send(t, ch.ID, strconv.Itoa(i), base.Add(time.Duration(i)*time.Second))
	}

	page, err := f.store.Messages(f.ctx, ch.ID, oldest.ID, 10)
	if err != nil {
		t.Fatalf("Messages: %v", err)
	}
	if len(page.Messages) != 0 {
		t.Errorf("expected no messages older than an evicted cursor, got %v", bodies(page.Messages))
	}
	if !page.Truncated {
		t.Error("expected Truncated when the cursor itself was forgotten")
	}
}

func TestMessagesRejectsAMalformedCursor(t *testing.T) {
	t.Parallel()

	f := newFixture(t, 4, time.Hour)
	ch := f.channel(t, "?teste")

	_, err := f.store.Messages(f.ctx, ch.ID, "not-a-ulid", 10)
	if got := chat.ReasonOf(err); got != chat.ReasonCursorInvalid {
		t.Errorf("reason = %s, want %s", got, chat.ReasonCursorInvalid)
	}
}

func TestAppendIsIdempotentOnClientID(t *testing.T) {
	t.Parallel()

	f := newFixture(t, 8, time.Hour)
	ch := f.channel(t, "?teste")

	first := f.send(t, ch.ID, "hello", base)

	retry := chat.Message{
		ID:        f.id(t, base.Add(time.Second)),
		ClientID:  first.ClientID,
		ChannelID: ch.ID,
		Author:    "rill",
		Body:      "hello",
		Kind:      chat.KindNormal,
		CreatedAt: base.Add(time.Second),
	}
	stored, deduplicated, err := f.store.Append(f.ctx, retry)
	if err != nil {
		t.Fatalf("Append: %v", err)
	}
	if !deduplicated {
		t.Error("expected the retry to be reported as deduplicated")
	}
	if stored.ID != first.ID {
		t.Errorf("returned id = %s, want the original %s", stored.ID, first.ID)
	}

	page, err := f.store.Messages(f.ctx, ch.ID, "", 10)
	if err != nil {
		t.Fatalf("Messages: %v", err)
	}
	if len(page.Messages) != 1 {
		t.Errorf("channel holds %d messages, want 1", len(page.Messages))
	}
}

func TestAppendRejectsDirectMessages(t *testing.T) {
	t.Parallel()

	f := newFixture(t, 4, time.Hour)
	ch := f.channel(t, "?teste")

	_, _, err := f.store.Append(f.ctx, chat.Message{
		ID:        f.id(t, base),
		ChannelID: ch.ID,
		Kind:      chat.KindDirect,
		Body:      "psst",
		CreatedAt: base,
	})
	if got := chat.ReasonOf(err); got != chat.ReasonMessageKindInvalid {
		t.Errorf("reason = %s, want %s", got, chat.ReasonMessageKindInvalid)
	}
}

func TestEnsureChannelCreatesOnceAndIsCaseNormalisedByCaller(t *testing.T) {
	t.Parallel()

	f := newFixture(t, 4, time.Hour)

	first, created, err := f.store.EnsureChannel(f.ctx, f.id(t, base), "?teste", base)
	if err != nil {
		t.Fatalf("EnsureChannel: %v", err)
	}
	if !created {
		t.Error("first EnsureChannel should report created")
	}

	second, created, err := f.store.EnsureChannel(f.ctx, f.id(t, base), "?teste", base)
	if err != nil {
		t.Fatalf("EnsureChannel: %v", err)
	}
	if created {
		t.Error("second EnsureChannel should not report created")
	}
	if second.ID != first.ID {
		t.Errorf("second id = %s, want %s", second.ID, first.ID)
	}
}

func TestNickIsUniqueAmongConnectedSessions(t *testing.T) {
	t.Parallel()

	f := newFixture(t, 4, time.Hour)

	if err := f.store.CreateSession(f.ctx, chat.Session{ID: "s1", Nick: "rill", ExpiresAt: base.Add(time.Hour)}); err != nil {
		t.Fatalf("first session: %v", err)
	}

	err := f.store.CreateSession(f.ctx, chat.Session{ID: "s2", Nick: "RILL", ExpiresAt: base.Add(time.Hour)})
	if got := chat.ReasonOf(err); got != chat.ReasonNickTaken {
		t.Errorf("reason = %s, want %s", got, chat.ReasonNickTaken)
	}

	if err := f.store.DeleteSession(f.ctx, "s1"); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}
	if err := f.store.CreateSession(f.ctx, chat.Session{ID: "s2", Nick: "rill", ExpiresAt: base.Add(time.Hour)}); err != nil {
		t.Errorf("nick should be reusable once released: %v", err)
	}
}

func TestNickRaceProducesExactlyOneWinner(t *testing.T) {
	t.Parallel()

	f := newFixture(t, 4, time.Hour)

	const racers = 50

	var (
		wg    sync.WaitGroup
		mu    sync.Mutex
		wins  int
		start = make(chan struct{})
	)

	wg.Add(racers)
	for i := range racers {
		go func() {
			defer wg.Done()
			<-start
			err := f.store.CreateSession(f.ctx, chat.Session{
				ID:        "s" + strconv.Itoa(i),
				Nick:      "rill",
				ExpiresAt: base.Add(time.Hour),
			})
			if err == nil {
				mu.Lock()
				wins++
				mu.Unlock()
			}
		}()
	}
	close(start)
	wg.Wait()

	if wins != 1 {
		t.Errorf("%d sessions claimed the same nick, want exactly 1", wins)
	}
}

func TestSetNickReleasesTheOldNickAndRenamesMemberships(t *testing.T) {
	t.Parallel()

	f := newFixture(t, 4, time.Hour)
	ch := f.channel(t, "?teste")

	if err := f.store.CreateSession(f.ctx, chat.Session{ID: "s1", Nick: "rill", ExpiresAt: base.Add(time.Hour)}); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	f.join(t, ch.ID, "s1", "rill")

	if _, err := f.store.SetNick(f.ctx, "s1", "0x9c"); err != nil {
		t.Fatalf("SetNick: %v", err)
	}

	members, err := f.store.Members(f.ctx, ch.ID)
	if err != nil {
		t.Fatalf("Members: %v", err)
	}
	if len(members) != 1 || members[0].Nick != "0x9c" {
		t.Errorf("roster = %+v, want the renamed member", members)
	}

	if err := f.store.CreateSession(f.ctx, chat.Session{ID: "s2", Nick: "rill", ExpiresAt: base.Add(time.Hour)}); err != nil {
		t.Errorf("old nick should have been released: %v", err)
	}
}

func TestDeleteSessionLeavesEveryChannel(t *testing.T) {
	t.Parallel()

	f := newFixture(t, 4, time.Hour)
	first := f.channel(t, "?teste")
	second := f.channel(t, "?lounge")

	if err := f.store.CreateSession(f.ctx, chat.Session{ID: "s1", Nick: "rill", ExpiresAt: base.Add(time.Hour)}); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	f.join(t, first.ID, "s1", "rill")
	f.join(t, second.ID, "s1", "rill")

	if err := f.store.DeleteSession(f.ctx, "s1"); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}

	for _, ch := range []chat.Channel{first, second} {
		members, err := f.store.Members(f.ctx, ch.ID)
		if err != nil {
			t.Fatalf("Members(%s): %v", ch.Name, err)
		}
		if len(members) != 0 {
			t.Errorf("%s still holds %d members", ch.Name, len(members))
		}
	}

	channels, err := f.store.MemberChannels(f.ctx, "s1")
	if err != nil {
		t.Fatalf("MemberChannels: %v", err)
	}
	if len(channels) != 0 {
		t.Errorf("session still maps to %v", channels)
	}
}

func TestUnknownChannelIsReported(t *testing.T) {
	t.Parallel()

	f := newFixture(t, 4, time.Hour)

	_, err := f.store.Messages(f.ctx, "nope", "", 10)
	if got := chat.ReasonOf(err); got != chat.ReasonChannelNotFound {
		t.Errorf("reason = %s, want %s", got, chat.ReasonChannelNotFound)
	}
}

func TestConcurrentAppendsAreRaceFree(t *testing.T) {
	t.Parallel()

	f := newFixture(t, 64, time.Hour)
	ch := f.channel(t, "?teste")

	const writers, each = 8, 50

	var wg sync.WaitGroup
	wg.Add(writers)
	for w := range writers {
		go func() {
			defer wg.Done()
			for i := range each {
				body := strconv.Itoa(w) + "-" + strconv.Itoa(i)
				id, err := f.gen.New(base)
				if err != nil {
					t.Errorf("id: %v", err)
					return
				}
				_, _, err = f.store.Append(f.ctx, chat.Message{
					ID:        id,
					ClientID:  body,
					ChannelID: ch.ID,
					Author:    "rill",
					Body:      body,
					Kind:      chat.KindNormal,
					CreatedAt: base,
				})
				if err != nil {
					t.Errorf("Append: %v", err)
					return
				}
			}
		}()
	}
	wg.Wait()

	page, err := f.store.Messages(f.ctx, ch.ID, "", 100)
	if err != nil {
		t.Fatalf("Messages: %v", err)
	}
	if len(page.Messages) != 64 {
		t.Errorf("ring holds %d messages, want its capacity of 64", len(page.Messages))
	}
}
