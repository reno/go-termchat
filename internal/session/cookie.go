package session

import (
	"net/http"
	"time"
)

const DefaultCookieName = "termchat_session"

type Cookies struct {
	name string
}

func NewCookies(name string) Cookies {
	if name == "" {
		name = DefaultCookieName
	}
	return Cookies{name: name}
}

func (c Cookies) Name() string {
	return c.name
}

func (c Cookies) Issue(token string, expires time.Time) *http.Cookie {
	return &http.Cookie{
		Name:     c.name,
		Value:    token,
		Path:     "/",
		Expires:  expires,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	}
}

func (c Cookies) Revoke() *http.Cookie {
	return &http.Cookie{
		Name:     c.name,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	}
}

func (c Cookies) Token(header http.Header) string {
	raw := header.Get("Cookie")
	if raw == "" {
		return ""
	}
	cookies, err := http.ParseCookie(raw)
	if err != nil {
		return ""
	}
	for _, cookie := range cookies {
		if cookie.Name == c.name {
			return cookie.Value
		}
	}
	return ""
}

func (c Cookies) Present(header http.Header) bool {
	for _, raw := range header.Values("Set-Cookie") {
		cookie, err := http.ParseSetCookie(raw)
		if err == nil && cookie.Name == c.name {
			return true
		}
	}
	return false
}

func Attach(header http.Header, cookie *http.Cookie) {
	header.Add("Set-Cookie", cookie.String())
}
