package session_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/reno/go-termchat/internal/session"
)

func TestIssuedCookieCarriesEveryRequiredFlag(t *testing.T) {
	t.Parallel()

	expires := time.Date(2026, time.August, 14, 12, 30, 0, 0, time.UTC)
	cookie := session.NewCookies("").Issue("token-value", expires)

	if cookie.Name != session.DefaultCookieName {
		t.Errorf("Name = %q, want %q", cookie.Name, session.DefaultCookieName)
	}
	if cookie.Value != "token-value" {
		t.Errorf("Value = %q", cookie.Value)
	}
	if !cookie.HttpOnly {
		t.Error("HttpOnly is not set, the token would be readable from script")
	}
	if !cookie.Secure {
		t.Error("Secure is not set")
	}
	if cookie.SameSite != http.SameSiteLaxMode {
		t.Errorf("SameSite = %v, want Lax", cookie.SameSite)
	}
	if cookie.Path != "/" {
		t.Errorf("Path = %q, want /", cookie.Path)
	}
	if !cookie.Expires.Equal(expires) {
		t.Errorf("Expires = %s, want %s", cookie.Expires, expires)
	}
}

func TestRevokedCookieExpiresImmediatelyAndKeepsItsFlags(t *testing.T) {
	t.Parallel()

	cookie := session.NewCookies("").Revoke()

	if cookie.Value != "" {
		t.Errorf("Value = %q, want empty", cookie.Value)
	}
	if cookie.MaxAge >= 0 {
		t.Errorf("MaxAge = %d, want negative so the browser drops it", cookie.MaxAge)
	}
	if !cookie.HttpOnly || !cookie.Secure || cookie.SameSite != http.SameSiteLaxMode {
		t.Error("revocation must repeat the flags or the browser will not match the cookie")
	}
}

func TestCookieNameIsConfigurable(t *testing.T) {
	t.Parallel()

	cookies := session.NewCookies("custom_name")
	if cookies.Name() != "custom_name" {
		t.Errorf("Name = %q", cookies.Name())
	}
	if got := cookies.Issue("v", time.Now()).Name; got != "custom_name" {
		t.Errorf("issued cookie name = %q", got)
	}
}

func TestTokenReadsOnlyItsOwnCookie(t *testing.T) {
	t.Parallel()

	cookies := session.NewCookies("")

	tests := []struct {
		name   string
		header string
		want   string
	}{
		{name: "absent", header: "", want: ""},
		{name: "present alone", header: "termchat_session=abc", want: "abc"},
		{name: "among others", header: "a=1; termchat_session=abc; b=2", want: "abc"},
		{name: "different name", header: "other=abc", want: ""},
		{name: "prefix collision", header: "termchat_session_x=abc", want: ""},
		{name: "malformed", header: "=;;", want: ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			header := http.Header{}
			if tc.header != "" {
				header.Set("Cookie", tc.header)
			}
			if got := cookies.Token(header); got != tc.want {
				t.Errorf("Token = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestPresentDetectsAnAlreadyIssuedCookie(t *testing.T) {
	t.Parallel()

	cookies := session.NewCookies("")
	header := http.Header{}

	if cookies.Present(header) {
		t.Error("Present on an empty header")
	}

	session.Attach(header, cookies.Revoke())
	if !cookies.Present(header) {
		t.Error("Present missed a cookie this package attached")
	}

	other := http.Header{}
	session.Attach(other, &http.Cookie{Name: "unrelated", Value: "x"})
	if cookies.Present(other) {
		t.Error("Present matched an unrelated cookie")
	}
}
