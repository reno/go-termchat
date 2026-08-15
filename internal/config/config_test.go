package config_test

import (
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
	if want := config.Default(); got != want {
		t.Errorf("Load = %+v, want %+v", got, want)
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
