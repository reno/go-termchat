package chat

import (
	"errors"
	"strconv"
	"time"
)

type Reason uint8

const (
	ReasonUnspecified Reason = iota
	ReasonNickTaken
	ReasonNickInvalid
	ReasonChannelNameInvalid
	ReasonBodyTooLong
	ReasonBodyEmpty
	ReasonNotAMember
	ReasonChannelNotFound
	ReasonRateLimited
	ReasonSessionExpired
	ReasonRecipientOffline
	ReasonCursorInvalid
	ReasonEventsEvicted
	ReasonMessageKindInvalid
	ReasonClientMessageIDInvalid
)

var reasonMessages = map[Reason]string{
	ReasonUnspecified:            "unspecified failure",
	ReasonNickTaken:              "nick already taken",
	ReasonNickInvalid:            "nick is not valid",
	ReasonChannelNameInvalid:     "channel name is not valid",
	ReasonBodyTooLong:            "message body is too long",
	ReasonBodyEmpty:              "message body is empty",
	ReasonNotAMember:             "not a member of that channel",
	ReasonChannelNotFound:        "no such channel",
	ReasonRateLimited:            "sending too fast",
	ReasonSessionExpired:         "session has expired",
	ReasonRecipientOffline:       "recipient is not connected",
	ReasonCursorInvalid:          "cursor is not valid",
	ReasonEventsEvicted:          "events have been forgotten",
	ReasonMessageKindInvalid:     "message kind is not one a client may author",
	ReasonClientMessageIDInvalid: "client message id is missing or not a ulid",
}

func (r Reason) String() string {
	if msg, ok := reasonMessages[r]; ok {
		return msg
	}
	return reasonMessages[ReasonUnspecified]
}

type Error struct {
	Reason     Reason
	Field      string
	Limit      int
	RetryAfter time.Duration
}

func (e *Error) Error() string {
	msg := e.Reason.String()
	switch {
	case e.Field != "" && e.Limit > 0:
		return msg + ": " + e.Field + " limit " + strconv.Itoa(e.Limit)
	case e.Field != "":
		return msg + ": " + e.Field
	case e.Limit > 0:
		return msg + ": limit " + strconv.Itoa(e.Limit)
	default:
		return msg
	}
}

func (e *Error) Is(target error) bool {
	var other *Error
	if !errors.As(target, &other) {
		return false
	}
	return e.Reason == other.Reason
}

func Fail(reason Reason, field string) *Error {
	return &Error{Reason: reason, Field: field}
}

func FailLimit(reason Reason, field string, limit int) *Error {
	return &Error{Reason: reason, Field: field, Limit: limit}
}

func ReasonOf(err error) Reason {
	var domain *Error
	if errors.As(err, &domain) {
		return domain.Reason
	}
	return ReasonUnspecified
}
