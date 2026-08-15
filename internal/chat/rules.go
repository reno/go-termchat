package chat

import (
	"strings"
	"unicode/utf8"
)

const ChannelPrefix = '?'

type Limits struct {
	NickMaxLen        int
	ChannelNameMaxLen int
	BodyMaxLen        int
}

func DefaultLimits() Limits {
	return Limits{
		NickMaxLen:        24,
		ChannelNameMaxLen: 32,
		BodyMaxLen:        2000,
	}
}

type Rules struct {
	limits Limits
}

func NewRules(limits Limits) Rules {
	return Rules{limits: limits}
}

func (r Rules) Limits() Limits {
	return r.limits
}

func (r Rules) NormalizeNick(raw string) (string, error) {
	nick := strings.TrimSpace(raw)
	if nick == "" {
		return "", Fail(ReasonNickInvalid, "nick")
	}
	if utf8.RuneCountInString(nick) > r.limits.NickMaxLen {
		return "", FailLimit(ReasonNickInvalid, "nick", r.limits.NickMaxLen)
	}
	if !allowedName(nick) {
		return "", Fail(ReasonNickInvalid, "nick")
	}
	return nick, nil
}

func (r Rules) NormalizeChannelName(raw string) (string, error) {
	name := strings.ToLower(strings.TrimSpace(raw))
	name = strings.TrimPrefix(name, string(ChannelPrefix))
	if name == "" {
		return "", Fail(ReasonChannelNameInvalid, "name")
	}
	if utf8.RuneCountInString(name) > r.limits.ChannelNameMaxLen {
		return "", FailLimit(ReasonChannelNameInvalid, "name", r.limits.ChannelNameMaxLen)
	}
	if !allowedName(name) {
		return "", Fail(ReasonChannelNameInvalid, "name")
	}
	return string(ChannelPrefix) + name, nil
}

func (r Rules) NormalizeBody(raw string) (string, error) {
	body := strings.TrimRight(raw, " \t\r\n")
	if strings.TrimSpace(body) == "" {
		return "", Fail(ReasonBodyEmpty, "body")
	}
	if utf8.RuneCountInString(body) > r.limits.BodyMaxLen {
		return "", FailLimit(ReasonBodyTooLong, "body", r.limits.BodyMaxLen)
	}
	return body, nil
}

func (r Rules) AuthorizePost(isMember bool, kind MessageKind) error {
	if kind != KindNormal && kind != KindAction {
		return Fail(ReasonMessageKindInvalid, "kind")
	}
	if !isMember {
		return Fail(ReasonNotAMember, "channel_id")
	}
	return nil
}

func FoldNick(nick string) string {
	return strings.ToLower(strings.TrimSpace(nick))
}

func allowedName(s string) bool {
	alnum := false
	for _, c := range s {
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
			alnum = true
		case c == '_' || c == '-':
		default:
			return false
		}
	}
	return alnum
}
