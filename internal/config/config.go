package config

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/reno/go-termchat/internal/chat"
)

type Config struct {
	Addr      string
	Retention Retention
	Limits    chat.Limits
}

type Retention struct {
	MessagesPerChannel int
	MessageTTL         time.Duration
	SweepInterval      time.Duration
}

func Default() Config {
	return Config{
		Addr: ":8080",
		Retention: Retention{
			MessagesPerChannel: 200,
			MessageTTL:         time.Hour,
			SweepInterval:      time.Minute,
		},
		Limits: chat.DefaultLimits(),
	}
}

func Load(getenv func(string) string) (Config, error) {
	cfg := Default()
	var err error

	if cfg.Addr, err = str(getenv, "TERMCHAT_ADDR", cfg.Addr); err != nil {
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

	return cfg, nil
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
