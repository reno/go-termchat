package config_test

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"strings"
	"testing"
	"time"

	"github.com/reno/go-termchat/internal/config"
)

func env(pairs map[string]string) func(string) string {
	return func(key string) string { return pairs[key] }
}

func TestLoadFallsBackToDefaults(t *testing.T) {
	t.Parallel()

	got, err := config.Load(env(nil))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	want := config.Default()
	if got.Env != want.Env {
		t.Errorf("Env = %q, want %q", got.Env, want.Env)
	}
	if got.Addr != want.Addr {
		t.Errorf("Addr = %q, want %q", got.Addr, want.Addr)
	}
	if got.Retention != want.Retention {
		t.Errorf("Retention = %+v, want %+v", got.Retention, want.Retention)
	}
	if got.Limits != want.Limits {
		t.Errorf("Limits = %+v, want %+v", got.Limits, want.Limits)
	}
	if got.Session.TTL != want.Session.TTL {
		t.Errorf("Session.TTL = %s, want %s", got.Session.TTL, want.Session.TTL)
	}
	if got.Session.CookieName != want.Session.CookieName {
		t.Errorf("Session.CookieName = %q, want %q", got.Session.CookieName, want.Session.CookieName)
	}
}

func TestDevelopmentGeneratesASessionKey(t *testing.T) {
	t.Parallel()

	got, err := config.Load(env(nil))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !got.Session.Generated {
		t.Error("Generated is false, the caller cannot warn that sessions die on restart")
	}
	if len(got.Session.Key) < config.MinSessionKeyLen {
		t.Errorf("generated key is %d bytes, want at least %d",
			len(got.Session.Key), config.MinSessionKeyLen)
	}

	other, err := config.Load(env(nil))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if bytes.Equal(got.Session.Key, other.Session.Key) {
		t.Error("two boots generated the same key")
	}
}

func TestProductionRefusesToGenerateASessionKey(t *testing.T) {
	t.Parallel()

	_, err := config.Load(env(map[string]string{"TERMCHAT_ENV": "production"}))
	if err == nil {
		t.Fatal("production booted without TERMCHAT_SESSION_KEY")
	}
	if !strings.Contains(err.Error(), "TERMCHAT_SESSION_KEY") {
		t.Errorf("error does not name the missing variable: %v", err)
	}
}

func TestProductionBootsWithAKey(t *testing.T) {
	t.Parallel()

	key := strings.Repeat("ab", config.MinSessionKeyLen)
	got, err := config.Load(env(map[string]string{
		"TERMCHAT_ENV":         "production",
		"TERMCHAT_SESSION_KEY": key,
	}))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !got.Production() {
		t.Error("Production() is false")
	}
	if got.Session.Generated {
		t.Error("Generated is true for a supplied key")
	}
	if len(got.Session.Key) != config.MinSessionKeyLen {
		t.Errorf("key decoded to %d bytes, want %d", len(got.Session.Key), config.MinSessionKeyLen)
	}
}

func TestSessionKeyAcceptsHexAndBase64(t *testing.T) {
	t.Parallel()

	raw := bytes.Repeat([]byte{0x2a}, config.MinSessionKeyLen)

	tests := []struct {
		name    string
		encoded string
	}{
		{name: "hex", encoded: hex.EncodeToString(raw)},
		{name: "base64 std", encoded: base64.StdEncoding.EncodeToString(raw)},
		{name: "base64 raw url", encoded: base64.RawURLEncoding.EncodeToString(raw)},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := config.Load(env(map[string]string{"TERMCHAT_SESSION_KEY": tc.encoded}))
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if !bytes.Equal(got.Session.Key, raw) {
				t.Errorf("key = %x, want %x", got.Session.Key, raw)
			}
		})
	}
}

func TestSessionKeyRejectsWeakOrUndecodableValues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		encoded string
	}{
		{name: "too short in hex", encoded: hex.EncodeToString(bytes.Repeat([]byte{1}, 16))},
		{name: "too short in base64", encoded: base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{1}, 16))},
		{name: "not an encoding", encoded: "this is not a key!!"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if _, err := config.Load(env(map[string]string{"TERMCHAT_SESSION_KEY": tc.encoded})); err == nil {
				t.Error("expected an error, got nil")
			}
		})
	}
}

func TestEnvMustBeKnown(t *testing.T) {
	t.Parallel()

	if _, err := config.Load(env(map[string]string{"TERMCHAT_ENV": "staging"})); err == nil {
		t.Error("unknown environment accepted")
	}
	if _, err := config.Load(env(map[string]string{"TERMCHAT_ENV": "PRODUCTION", "TERMCHAT_SESSION_KEY": strings.Repeat("ab", 32)})); err != nil {
		t.Errorf("environment should be case-insensitive: %v", err)
	}
}

func TestLoadReadsEveryKnob(t *testing.T) {
	t.Parallel()

	got, err := config.Load(env(map[string]string{
		"TERMCHAT_ADDR":                 "127.0.0.1:9000",
		"TERMCHAT_MESSAGES_PER_CHANNEL": "500",
		"TERMCHAT_MESSAGE_TTL":          "15m",
		"TERMCHAT_SWEEP_INTERVAL":       "5s",
		"TERMCHAT_NICK_MAX_LEN":         "12",
		"TERMCHAT_CHANNEL_NAME_MAX_LEN": "20",
		"TERMCHAT_BODY_MAX_LEN":         "500",
		"TERMCHAT_SESSION_TTL":          "10m",
	}))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if got.Addr != "127.0.0.1:9000" {
		t.Errorf("Addr = %q", got.Addr)
	}
	if got.Retention.MessagesPerChannel != 500 {
		t.Errorf("MessagesPerChannel = %d", got.Retention.MessagesPerChannel)
	}
	if got.Retention.MessageTTL != 15*time.Minute {
		t.Errorf("MessageTTL = %s", got.Retention.MessageTTL)
	}
	if got.Retention.SweepInterval != 5*time.Second {
		t.Errorf("SweepInterval = %s", got.Retention.SweepInterval)
	}
	if got.Limits.NickMaxLen != 12 {
		t.Errorf("NickMaxLen = %d", got.Limits.NickMaxLen)
	}
	if got.Limits.ChannelNameMaxLen != 20 {
		t.Errorf("ChannelNameMaxLen = %d", got.Limits.ChannelNameMaxLen)
	}
	if got.Limits.BodyMaxLen != 500 {
		t.Errorf("BodyMaxLen = %d", got.Limits.BodyMaxLen)
	}
	if got.Session.TTL != 10*time.Minute {
		t.Errorf("Session.TTL = %s", got.Session.TTL)
	}
}

func TestLoadRejectsBadValues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		vars map[string]string
	}{
		{name: "empty addr", vars: map[string]string{"TERMCHAT_ADDR": " "}},
		{name: "capacity not a number", vars: map[string]string{"TERMCHAT_MESSAGES_PER_CHANNEL": "lots"}},
		{name: "capacity zero", vars: map[string]string{"TERMCHAT_MESSAGES_PER_CHANNEL": "0"}},
		{name: "capacity negative", vars: map[string]string{"TERMCHAT_MESSAGES_PER_CHANNEL": "-1"}},
		{name: "ttl not a duration", vars: map[string]string{"TERMCHAT_MESSAGE_TTL": "forever"}},
		{name: "ttl zero", vars: map[string]string{"TERMCHAT_MESSAGE_TTL": "0s"}},
		{name: "ttl negative", vars: map[string]string{"TERMCHAT_MESSAGE_TTL": "-1m"}},
		{name: "sweep interval zero", vars: map[string]string{"TERMCHAT_SWEEP_INTERVAL": "0s"}},
		{name: "body cap zero", vars: map[string]string{"TERMCHAT_BODY_MAX_LEN": "0"}},
		{name: "nick cap negative", vars: map[string]string{"TERMCHAT_NICK_MAX_LEN": "-3"}},
		{name: "session ttl zero", vars: map[string]string{"TERMCHAT_SESSION_TTL": "0s"}},
		{name: "session ttl blank", vars: map[string]string{"TERMCHAT_SESSION_TTL": "  "}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if _, err := config.Load(env(tc.vars)); err == nil {
				t.Error("expected an error, got nil")
			}
		})
	}
}
