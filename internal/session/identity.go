package session

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
)

const (
	MinKeyLen  = 32
	idBytes    = 16
	separator  = "."
	encodedLen = 22
)

var (
	ErrKeyTooShort  = errors.New("session: signing key must be at least 32 bytes")
	ErrInvalidToken = errors.New("session: token is not valid")
)

type Signer struct {
	key []byte
}

func NewSigner(key []byte) (*Signer, error) {
	if len(key) < MinKeyLen {
		return nil, ErrKeyTooShort
	}
	owned := make([]byte, len(key))
	copy(owned, key)
	return &Signer{key: owned}, nil
}

func (s *Signer) Mint() (id, token string, err error) {
	raw := make([]byte, idBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", "", fmt.Errorf("session: mint id: %w", err)
	}
	id = base64.RawURLEncoding.EncodeToString(raw)
	return id, s.Token(id), nil
}

func (s *Signer) Token(id string) string {
	return id + separator + base64.RawURLEncoding.EncodeToString(s.mac(id))
}

func (s *Signer) Verify(token string) (string, error) {
	id, encoded, found := strings.Cut(token, separator)
	if !found || len(id) != encodedLen {
		return "", ErrInvalidToken
	}
	presented, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return "", ErrInvalidToken
	}
	if !hmac.Equal(presented, s.mac(id)) {
		return "", ErrInvalidToken
	}
	return id, nil
}

func (s *Signer) mac(id string) []byte {
	mac := hmac.New(sha256.New, s.key)
	mac.Write([]byte(id))
	return mac.Sum(nil)
}
