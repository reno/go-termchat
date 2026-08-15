package chat

import (
	"crypto/rand"
	"fmt"
	"sync"
	"time"

	"github.com/oklog/ulid/v2"
)

type IDGen struct {
	mu      sync.Mutex
	entropy *ulid.MonotonicEntropy
}

func NewIDGen() *IDGen {
	return &IDGen{entropy: ulid.Monotonic(rand.Reader, 0)}
}

func (g *IDGen) New(at time.Time) (string, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	id, err := ulid.New(ulid.Timestamp(at), g.entropy)
	if err != nil {
		return "", fmt.Errorf("chat: new id: %w", err)
	}
	return id.String(), nil
}

func ValidID(s string) bool {
	_, err := ulid.ParseStrict(s)
	return err == nil
}

func IDTime(s string) (time.Time, error) {
	id, err := ulid.ParseStrict(s)
	if err != nil {
		return time.Time{}, fmt.Errorf("chat: parse id: %w", err)
	}
	return ulid.Time(id.Time()), nil
}
