package rpc

import (
	"errors"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/durationpb"

	chatv1 "github.com/reno/go-termchat/gen/go/chat/v1"
	"github.com/reno/go-termchat/internal/chat"
)

var reasonCodes = map[chat.Reason]connect.Code{
	chat.ReasonUnspecified:            connect.CodeInternal,
	chat.ReasonNickTaken:              connect.CodeAlreadyExists,
	chat.ReasonNickInvalid:            connect.CodeInvalidArgument,
	chat.ReasonChannelNameInvalid:     connect.CodeInvalidArgument,
	chat.ReasonBodyTooLong:            connect.CodeInvalidArgument,
	chat.ReasonBodyEmpty:              connect.CodeInvalidArgument,
	chat.ReasonNotAMember:             connect.CodePermissionDenied,
	chat.ReasonChannelNotFound:        connect.CodeNotFound,
	chat.ReasonRateLimited:            connect.CodeResourceExhausted,
	chat.ReasonSessionExpired:         connect.CodeUnauthenticated,
	chat.ReasonRecipientOffline:       connect.CodeFailedPrecondition,
	chat.ReasonCursorInvalid:          connect.CodeInvalidArgument,
	chat.ReasonEventsEvicted:          connect.CodeFailedPrecondition,
	chat.ReasonMessageKindInvalid:     connect.CodeInvalidArgument,
	chat.ReasonClientMessageIDInvalid: connect.CodeInvalidArgument,
}

var reasonProtos = map[chat.Reason]chatv1.ErrorReason{
	chat.ReasonUnspecified:            chatv1.ErrorReason_ERROR_REASON_UNSPECIFIED,
	chat.ReasonNickTaken:              chatv1.ErrorReason_ERROR_REASON_NICK_TAKEN,
	chat.ReasonNickInvalid:            chatv1.ErrorReason_ERROR_REASON_NICK_INVALID,
	chat.ReasonChannelNameInvalid:     chatv1.ErrorReason_ERROR_REASON_CHANNEL_NAME_INVALID,
	chat.ReasonBodyTooLong:            chatv1.ErrorReason_ERROR_REASON_BODY_TOO_LONG,
	chat.ReasonBodyEmpty:              chatv1.ErrorReason_ERROR_REASON_BODY_EMPTY,
	chat.ReasonNotAMember:             chatv1.ErrorReason_ERROR_REASON_NOT_A_MEMBER,
	chat.ReasonChannelNotFound:        chatv1.ErrorReason_ERROR_REASON_CHANNEL_NOT_FOUND,
	chat.ReasonRateLimited:            chatv1.ErrorReason_ERROR_REASON_RATE_LIMITED,
	chat.ReasonSessionExpired:         chatv1.ErrorReason_ERROR_REASON_SESSION_EXPIRED,
	chat.ReasonRecipientOffline:       chatv1.ErrorReason_ERROR_REASON_RECIPIENT_OFFLINE,
	chat.ReasonCursorInvalid:          chatv1.ErrorReason_ERROR_REASON_CURSOR_INVALID,
	chat.ReasonEventsEvicted:          chatv1.ErrorReason_ERROR_REASON_EVENTS_EVICTED,
	chat.ReasonMessageKindInvalid:     chatv1.ErrorReason_ERROR_REASON_MESSAGE_KIND_INVALID,
	chat.ReasonClientMessageIDInvalid: chatv1.ErrorReason_ERROR_REASON_CLIENT_MESSAGE_ID_INVALID,
}

func Error(err error) error {
	if err == nil {
		return nil
	}

	var already *connect.Error
	if errors.As(err, &already) {
		return already
	}

	var domain *chat.Error
	if !errors.As(err, &domain) {
		return connect.NewError(connect.CodeInternal, err)
	}

	code, ok := reasonCodes[domain.Reason]
	if !ok {
		code = connect.CodeInternal
	}

	wrapped := connect.NewError(code, errors.New(domain.Reason.String()))
	if detail, detailErr := connect.NewErrorDetail(errorDetail(domain)); detailErr == nil {
		wrapped.AddDetail(detail)
	}
	return wrapped
}

func errorDetail(domain *chat.Error) *chatv1.ErrorDetail {
	detail := &chatv1.ErrorDetail{
		Reason: reasonProtos[domain.Reason],
		Field:  domain.Field,
	}
	if domain.Limit > 0 {
		detail.Limit = uint32(domain.Limit) //nolint:gosec // limits come from config and are small positive ints
	}
	if domain.RetryAfter > 0 {
		detail.RetryAfter = durationpb.New(domain.RetryAfter)
	}
	return detail
}
