package session_test

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/reno/go-termchat/internal/session"
)

func testKey() []byte {
	return bytes.Repeat([]byte{0x2a}, session.MinKeyLen)
}

func newSigner(t *testing.T) *session.Signer {
	t.Helper()

	signer, err := session.NewSigner(testKey())
	if err != nil {
		t.Fatalf("NewSigner: %v", err)
	}
	return signer
}

func TestNewSignerRejectsAShortKey(t *testing.T) {
	t.Parallel()

	for _, size := range []int{0, 1, session.MinKeyLen - 1} {
		if _, err := session.NewSigner(bytes.Repeat([]byte{1}, size)); !errors.Is(err, session.ErrKeyTooShort) {
			t.Errorf("key of %d bytes accepted: %v", size, err)
		}
	}
	if _, err := session.NewSigner(testKey()); err != nil {
		t.Errorf("key of %d bytes rejected: %v", session.MinKeyLen, err)
	}
}

func TestSignerDoesNotAliasTheCallersKey(t *testing.T) {
	t.Parallel()

	key := testKey()
	signer, err := session.NewSigner(key)
	if err != nil {
		t.Fatalf("NewSigner: %v", err)
	}

	id, token, err := signer.Mint()
	if err != nil {
		t.Fatalf("Mint: %v", err)
	}

	for i := range key {
		key[i] = 0
	}

	got, err := signer.Verify(token)
	if err != nil {
		t.Fatalf("Verify after mutating the caller's key slice: %v", err)
	}
	if got != id {
		t.Errorf("id = %q, want %q", got, id)
	}
}

func TestMintProducesDistinctIDs(t *testing.T) {
	t.Parallel()

	signer := newSigner(t)
	seen := make(map[string]struct{}, 1000)

	for range 1000 {
		id, _, err := signer.Mint()
		if err != nil {
			t.Fatalf("Mint: %v", err)
		}
		if _, dup := seen[id]; dup {
			t.Fatalf("Mint repeated id %q", id)
		}
		seen[id] = struct{}{}
	}
}

func TestVerifyRoundTrips(t *testing.T) {
	t.Parallel()

	signer := newSigner(t)
	id, token, err := signer.Mint()
	if err != nil {
		t.Fatalf("Mint: %v", err)
	}

	got, err := signer.Verify(token)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if got != id {
		t.Errorf("Verify = %q, want %q", got, id)
	}
}

func TestVerifyRejectsTamperedTokens(t *testing.T) {
	t.Parallel()

	signer := newSigner(t)
	id, token, err := signer.Mint()
	if err != nil {
		t.Fatalf("Mint: %v", err)
	}
	signature := strings.TrimPrefix(token, id+".")

	other, err := session.NewSigner(bytes.Repeat([]byte{0x99}, session.MinKeyLen))
	if err != nil {
		t.Fatalf("NewSigner: %v", err)
	}
	_, foreign, err := other.Mint()
	if err != nil {
		t.Fatalf("Mint: %v", err)
	}

	tests := []struct {
		name  string
		token string
	}{
		{name: "empty", token: ""},
		{name: "no separator", token: id + signature},
		{name: "id only", token: id},
		{name: "signature only", token: "." + signature},
		{name: "flipped id", token: flip(id) + "." + signature},
		{name: "flipped signature", token: id + "." + flip(signature)},
		{name: "truncated signature", token: id + "." + signature[:len(signature)-4]},
		{name: "empty signature", token: id + "."},
		{name: "signature not base64", token: id + ".!!!!"},
		{name: "signed by another key", token: foreign},
		{name: "signature from another id", token: swapID(t, signer, id)},
		{name: "extra separator", token: token + ".extra"},
		{name: "padded id", token: " " + token},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if _, err := signer.Verify(tc.token); !errors.Is(err, session.ErrInvalidToken) {
				t.Errorf("Verify(%q) = %v, want ErrInvalidToken", tc.token, err)
			}
		})
	}
}

func swapID(t *testing.T, signer *session.Signer, id string) string {
	t.Helper()

	otherID, otherToken, err := signer.Mint()
	if err != nil {
		t.Fatalf("Mint: %v", err)
	}
	return id + "." + strings.TrimPrefix(otherToken, otherID+".")
}

func flip(s string) string {
	if s == "" {
		return s
	}
	swapped := []byte(s)
	if swapped[0] == 'A' {
		swapped[0] = 'B'
	} else {
		swapped[0] = 'A'
	}
	return string(swapped)
}
