package chat_test

import (
	"sync"
	"testing"
	"time"

	"github.com/reno/go-termchat/internal/chat"
)

func TestIDsSortChronologically(t *testing.T) {
	t.Parallel()

	gen := chat.NewIDGen()
	base := time.Date(2026, time.August, 14, 12, 0, 0, 0, time.UTC)

	var ids []string
	for i := range 5 {
		id, err := gen.New(base.Add(time.Duration(i) * time.Second))
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		ids = append(ids, id)
	}

	for i := 1; i < len(ids); i++ {
		if ids[i-1] >= ids[i] {
			t.Errorf("id %d (%s) not less than id %d (%s)", i-1, ids[i-1], i, ids[i])
		}
	}
}

func TestIDsAreMonotonicWithinTheSameInstant(t *testing.T) {
	t.Parallel()

	gen := chat.NewIDGen()
	at := time.Date(2026, time.August, 14, 12, 0, 0, 0, time.UTC)

	var ids []string
	for range 100 {
		id, err := gen.New(at)
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		ids = append(ids, id)
	}

	for i := 1; i < len(ids); i++ {
		if ids[i-1] >= ids[i] {
			t.Fatalf("ids not monotonic at %d: %s then %s", i, ids[i-1], ids[i])
		}
	}
}

func TestIDGenIsSafeUnderConcurrency(t *testing.T) {
	t.Parallel()

	gen := chat.NewIDGen()
	at := time.Now()

	const workers, each = 8, 200

	var (
		mu   sync.Mutex
		seen = make(map[string]struct{}, workers*each)
		wg   sync.WaitGroup
	)

	wg.Add(workers)
	for range workers {
		go func() {
			defer wg.Done()
			for range each {
				id, err := gen.New(at)
				if err != nil {
					t.Errorf("New: %v", err)
					return
				}
				mu.Lock()
				seen[id] = struct{}{}
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	if len(seen) != workers*each {
		t.Errorf("got %d distinct ids, want %d", len(seen), workers*each)
	}
}

func TestValidID(t *testing.T) {
	t.Parallel()

	id, err := chat.NewIDGen().New(time.Now())
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	tests := []struct {
		name string
		in   string
		want bool
	}{
		{name: "generated", in: id, want: true},
		{name: "empty", in: "", want: false},
		{name: "too short", in: "01J", want: false},
		{name: "not base32", in: "!!!!!!!!!!!!!!!!!!!!!!!!!!", want: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := chat.ValidID(tc.in); got != tc.want {
				t.Errorf("ValidID(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestIDTimeRoundTrips(t *testing.T) {
	t.Parallel()

	at := time.Date(2026, time.August, 14, 12, 0, 0, 0, time.UTC)
	id, err := chat.NewIDGen().New(at)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	got, err := chat.IDTime(id)
	if err != nil {
		t.Fatalf("IDTime: %v", err)
	}
	if !got.Equal(at) {
		t.Errorf("IDTime = %s, want %s", got, at)
	}
}
