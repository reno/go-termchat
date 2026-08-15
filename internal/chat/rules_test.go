package chat_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/reno/go-termchat/internal/chat"
)

func testRules() chat.Rules {
	return chat.NewRules(chat.Limits{NickMaxLen: 8, ChannelNameMaxLen: 10, BodyMaxLen: 20})
}

func TestNormalizeNick(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		in     string
		want   string
		reason chat.Reason
	}{
		{name: "plain", in: "rill", want: "rill"},
		{name: "digits", in: "0x9c", want: "0x9c"},
		{name: "mixed case preserved", in: "Rill", want: "Rill"},
		{name: "underscore and dash", in: "an_on-2", want: "an_on-2"},
		{name: "surrounding space trimmed", in: "  rill  ", want: "rill"},
		{name: "at max length", in: "abcdefgh", want: "abcdefgh"},
		{name: "empty", in: "", reason: chat.ReasonNickInvalid},
		{name: "whitespace only", in: "   ", reason: chat.ReasonNickInvalid},
		{name: "too long", in: "abcdefghi", reason: chat.ReasonNickInvalid},
		{name: "inner space", in: "ri ll", reason: chat.ReasonNickInvalid},
		{name: "punctuation", in: "ri!ll", reason: chat.ReasonNickInvalid},
		{name: "channel prefix", in: "?rill", reason: chat.ReasonNickInvalid},
		{name: "at sign", in: "@rill", reason: chat.ReasonNickInvalid},
		{name: "no alphanumeric", in: "___", reason: chat.ReasonNickInvalid},
		{name: "non ascii", in: "rïll", reason: chat.ReasonNickInvalid},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := testRules().NormalizeNick(tc.in)
			assertReason(t, err, tc.reason)
			if tc.reason == chat.ReasonUnspecified && got != tc.want {
				t.Errorf("NormalizeNick(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestNormalizeChannelName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		in     string
		want   string
		reason chat.Reason
	}{
		{name: "bare name gains prefix", in: "foo", want: "?foo"},
		{name: "prefix kept", in: "?foo", want: "?foo"},
		{name: "lowercased", in: "?Lounge", want: "?lounge"},
		{name: "trimmed", in: "  ?teste  ", want: "?teste"},
		{name: "underscore and dash", in: "off-top_2", want: "?off-top_2"},
		{name: "at max length", in: "abcdefghij", want: "?abcdefghij"},
		{name: "empty", in: "", reason: chat.ReasonChannelNameInvalid},
		{name: "prefix only", in: "?", reason: chat.ReasonChannelNameInvalid},
		{name: "too long", in: "abcdefghijk", reason: chat.ReasonChannelNameInvalid},
		{name: "inner space", in: "of f", reason: chat.ReasonChannelNameInvalid},
		{name: "double prefix", in: "??foo", reason: chat.ReasonChannelNameInvalid},
		{name: "slash", in: "foo/bar", reason: chat.ReasonChannelNameInvalid},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := testRules().NormalizeChannelName(tc.in)
			assertReason(t, err, tc.reason)
			if tc.reason == chat.ReasonUnspecified && got != tc.want {
				t.Errorf("NormalizeChannelName(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestNormalizeChannelNameIsIdempotent(t *testing.T) {
	t.Parallel()

	rules := testRules()
	once, err := rules.NormalizeChannelName("Foo")
	if err != nil {
		t.Fatalf("first pass: %v", err)
	}
	twice, err := rules.NormalizeChannelName(once)
	if err != nil {
		t.Fatalf("second pass: %v", err)
	}
	if once != twice {
		t.Errorf("not idempotent: %q then %q", once, twice)
	}
}

func TestNormalizeBody(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		in     string
		want   string
		reason chat.Reason
	}{
		{name: "plain", in: "the void, mostly", want: "the void, mostly"},
		{name: "trailing newline stripped", in: "hi\n\n", want: "hi"},
		{name: "inner newline kept", in: "one\ntwo", want: "one\ntwo"},
		{name: "leading space kept", in: "  indented", want: "  indented"},
		{name: "at max length", in: strings.Repeat("a", 20), want: strings.Repeat("a", 20)},
		{name: "empty", in: "", reason: chat.ReasonBodyEmpty},
		{name: "whitespace only", in: " \t\n ", reason: chat.ReasonBodyEmpty},
		{name: "too long", in: strings.Repeat("a", 21), reason: chat.ReasonBodyTooLong},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := testRules().NormalizeBody(tc.in)
			assertReason(t, err, tc.reason)
			if tc.reason == chat.ReasonUnspecified && got != tc.want {
				t.Errorf("NormalizeBody(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestBodyLengthIsCountedInRunes(t *testing.T) {
	t.Parallel()

	rules := chat.NewRules(chat.Limits{NickMaxLen: 8, ChannelNameMaxLen: 10, BodyMaxLen: 4})
	if _, err := rules.NormalizeBody("😀😀😀😀"); err != nil {
		t.Errorf("four runes rejected under a four rune cap: %v", err)
	}
	if _, err := rules.NormalizeBody("😀😀😀😀😀"); !errors.Is(err, chat.Fail(chat.ReasonBodyTooLong, "")) {
		t.Errorf("five runes accepted under a four rune cap: %v", err)
	}
}

func TestAuthorizePost(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		isMember bool
		kind     chat.MessageKind
		reason   chat.Reason
	}{
		{name: "member sends normal", isMember: true, kind: chat.KindNormal},
		{name: "member sends action", isMember: true, kind: chat.KindAction},
		{name: "member cannot author system", isMember: true, kind: chat.KindSystem, reason: chat.ReasonMessageKindInvalid},
		{name: "member cannot author direct", isMember: true, kind: chat.KindDirect, reason: chat.ReasonMessageKindInvalid},
		{name: "unspecified kind rejected", isMember: true, kind: chat.KindUnspecified, reason: chat.ReasonMessageKindInvalid},
		{name: "stranger rejected", isMember: false, kind: chat.KindNormal, reason: chat.ReasonNotAMember},
		{name: "kind checked before membership", isMember: false, kind: chat.KindSystem, reason: chat.ReasonMessageKindInvalid},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assertReason(t, testRules().AuthorizePost(tc.isMember, tc.kind), tc.reason)
		})
	}
}

func TestFoldNick(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in   string
		want string
	}{
		{in: "rill", want: "rill"},
		{in: "RILL", want: "rill"},
		{in: "Rill", want: "rill"},
		{in: "  Rill  ", want: "rill"},
	}

	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			t.Parallel()

			if got := chat.FoldNick(tc.in); got != tc.want {
				t.Errorf("FoldNick(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func assertReason(t *testing.T, err error, want chat.Reason) {
	t.Helper()

	if want == chat.ReasonUnspecified {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		return
	}
	if err == nil {
		t.Fatalf("expected %s, got nil", want)
	}
	if got := chat.ReasonOf(err); got != want {
		t.Fatalf("reason = %s, want %s", got, want)
	}
}
