package config

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/reno/go-termchat/internal/chat"
)

type Env string

const (
	EnvDevelopment Env = "development"
	EnvProduction  Env = "production"
)

const MinSessionKeyLen = 32

type Config struct {
	Env           Env
	Addr          string
	Session       Session
	Retention     Retention
	Limits        chat.Limits
	Rate          chat.Rate
	TypingTTL     time.Duration
	Keepalive     time.Duration
	EventBuffer   int
	CORSOrigins   []string
	ShutdownGrace time.Duration
}

type Session struct {
	Key        []byte
	TTL        time.Duration
	Generated  bool
	CookieName string
}

func (c Config) Production() bool {
	return c.Env == EnvProduction
}

type Retention struct {
	MessagesPerChannel int
	MessageTTL         time.Duration
	SweepInterval      time.Duration
}

func Default() Config {
	return Config{
		Env:  EnvDevelopment,
		Addr: ":8080",
		Session: Session{
			TTL:        30 * time.Minute,
			CookieName: "termchat_session",
		},
		Retention: Retention{
			MessagesPerChannel: 200,
			MessageTTL:         time.Hour,
			SweepInterval:      time.Minute,
		},
		Limits:        chat.DefaultLimits(),
		Rate:          chat.DefaultRate(),
		TypingTTL:     5 * time.Second,
		Keepalive:     20 * time.Second,
		EventBuffer:   64,
		ShutdownGrace: 10 * time.Second,
	}
}

func Load(getenv func(string) string) (Config, error) {
	cfg := Default()
	var err error

	if cfg.Addr, err = str(getenv, "TERMCHAT_ADDR", cfg.Addr); err != nil {
		return Config{}, err
	}
	if cfg.Env, err = environment(getenv); err != nil {
		return Config{}, err
	}
	if cfg.Session.TTL, err = span(
		getenv, "TERMCHAT_SESSION_TTL", cfg.Session.TTL,
	); err != nil {
		return Config{}, err
	}
	if cfg.Session.Key, cfg.Session.Generated, err = sessionKey(getenv, cfg.Production()); err != nil {
		return Config{}, err
	}

	if cfg.Retention.MessagesPerChannel, err = count(
		getenv, "TERMCHAT_MESSAGES_PER_CHANNEL", cfg.Retention.MessagesPerChannel,
	); err != nil {
		return Config{}, err
	}
	if cfg.Retention.MessageTTL, err = span(
		getenv, "TERMCHAT_MESSAGE_TTL", cfg.Retention.MessageTTL,
	); err != nil {
		return Config{}, err
	}
	if cfg.Retention.SweepInterval, err = span(
		getenv, "TERMCHAT_SWEEP_INTERVAL", cfg.Retention.SweepInterval,
	); err != nil {
		return Config{}, err
	}
	if cfg.Limits.NickMaxLen, err = count(
		getenv, "TERMCHAT_NICK_MAX_LEN", cfg.Limits.NickMaxLen,
	); err != nil {
		return Config{}, err
	}
	if cfg.Limits.ChannelNameMaxLen, err = count(
		getenv, "TERMCHAT_CHANNEL_NAME_MAX_LEN", cfg.Limits.ChannelNameMaxLen,
	); err != nil {
		return Config{}, err
	}
	if cfg.Limits.BodyMaxLen, err = count(
		getenv, "TERMCHAT_BODY_MAX_LEN", cfg.Limits.BodyMaxLen,
	); err != nil {
		return Config{}, err
	}
	if cfg.Rate.Burst, err = count(
		getenv, "TERMCHAT_SEND_BURST", cfg.Rate.Burst,
	); err != nil {
		return Config{}, err
	}
	if cfg.Rate.Refill, err = span(
		getenv, "TERMCHAT_SEND_REFILL", cfg.Rate.Refill,
	); err != nil {
		return Config{}, err
	}
	if cfg.Rate.Idle, err = span(
		getenv, "TERMCHAT_SEND_IDLE", cfg.Rate.Idle,
	); err != nil {
		return Config{}, err
	}
	if cfg.TypingTTL, err = span(
		getenv, "TERMCHAT_TYPING_TTL", cfg.TypingTTL,
	); err != nil {
		return Config{}, err
	}
	if cfg.Keepalive, err = span(
		getenv, "TERMCHAT_KEEPALIVE", cfg.Keepalive,
	); err != nil {
		return Config{}, err
	}
	if cfg.EventBuffer, err = count(
		getenv, "TERMCHAT_EVENT_BUFFER", cfg.EventBuffer,
	); err != nil {
		return Config{}, err
	}
	if cfg.ShutdownGrace, err = span(
		getenv, "TERMCHAT_SHUTDOWN_GRACE", cfg.ShutdownGrace,
	); err != nil {
		return Config{}, err
	}
	if cfg.CORSOrigins, err = list(getenv, "TERMCHAT_CORS_ORIGINS"); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func list(getenv func(string) string, key string) ([]string, error) {
	raw, set, err := lookup(getenv, key)
	if err != nil {
		return nil, err
	}
	if !set {
		return nil, nil
	}

	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		if trimmed == "*" {
			return nil, fmt.Errorf("config: %s: %q is not allowed, list origins explicitly", key, "*")
		}
		out = append(out, trimmed)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("config: %s: no usable origins", key)
	}
	return out, nil
}

func environment(getenv func(string) string) (Env, error) {
	raw, set, err := lookup(getenv, "TERMCHAT_ENV")
	if err != nil {
		return "", err
	}
	if !set {
		return EnvDevelopment, nil
	}
	switch Env(strings.ToLower(raw)) {
	case EnvDevelopment:
		return EnvDevelopment, nil
	case EnvProduction:
		return EnvProduction, nil
	default:
		return "", fmt.Errorf("config: %s: must be %q or %q, got %q",
			"TERMCHAT_ENV", EnvDevelopment, EnvProduction, raw)
	}
}

func sessionKey(getenv func(string) string, production bool) (key []byte, generated bool, err error) {
	raw, set, lookupErr := lookup(getenv, "TERMCHAT_SESSION_KEY")
	if lookupErr != nil {
		return nil, false, lookupErr
	}

	if !set {
		if production {
			return nil, false, fmt.Errorf(
				"config: %s: required in %s, refusing to generate one",
				"TERMCHAT_SESSION_KEY", EnvProduction,
			)
		}
		fresh := make([]byte, MinSessionKeyLen)
		if _, readErr := rand.Read(fresh); readErr != nil {
			return nil, false, fmt.Errorf("config: generate session key: %w", readErr)
		}
		return fresh, true, nil
	}

	decoded, decodeErr := decodeKey(raw)
	if decodeErr != nil {
		return nil, false, fmt.Errorf("config: %s: %w", "TERMCHAT_SESSION_KEY", decodeErr)
	}
	if len(decoded) < MinSessionKeyLen {
		return nil, false, fmt.Errorf("config: %s: must decode to at least %d bytes, got %d",
			"TERMCHAT_SESSION_KEY", MinSessionKeyLen, len(decoded))
	}
	return decoded, false, nil
}

func decodeKey(raw string) ([]byte, error) {
	if isHex(raw) {
		key, err := hex.DecodeString(raw)
		if err != nil {
			return nil, fmt.Errorf("decode hex: %w", err)
		}
		return key, nil
	}
	for _, enc := range []*base64.Encoding{
		base64.StdEncoding,
		base64.RawStdEncoding,
		base64.URLEncoding,
		base64.RawURLEncoding,
	} {
		if key, err := enc.DecodeString(raw); err == nil {
			return key, nil
		}
	}
	return nil, errors.New("must be hex or base64")
}

func isHex(raw string) bool {
	if raw == "" || len(raw)%2 != 0 {
		return false
	}
	for _, c := range raw {
		switch {
		case c >= '0' && c <= '9', c >= 'a' && c <= 'f', c >= 'A' && c <= 'F':
		default:
			return false
		}
	}
	return true
}

func lookup(getenv func(string) string, key string) (value string, set bool, err error) {
	raw := getenv(key)
	if raw == "" {
		return "", false, nil
	}
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", false, fmt.Errorf("config: %s: set but blank", key)
	}
	return trimmed, true, nil
}

func str(getenv func(string) string, key, fallback string) (string, error) {
	raw, set, err := lookup(getenv, key)
	if err != nil || !set {
		return fallback, err
	}
	return raw, nil
}

func count(getenv func(string) string, key string, fallback int) (int, error) {
	raw, set, err := lookup(getenv, key)
	if err != nil {
		return 0, err
	}
	if !set {
		return fallback, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("config: %s: %w", key, err)
	}
	if n <= 0 {
		return 0, fmt.Errorf("config: %s: must be positive, got %d", key, n)
	}
	return n, nil
}

func span(getenv func(string) string, key string, fallback time.Duration) (time.Duration, error) {
	raw, set, err := lookup(getenv, key)
	if err != nil {
		return 0, err
	}
	if !set {
		return fallback, nil
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("config: %s: %w", key, err)
	}
	if d <= 0 {
		return 0, fmt.Errorf("config: %s: must be positive, got %s", key, d)
	}
	return d, nil
}
