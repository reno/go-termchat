package rpc

import (
	"context"

	"connectrpc.com/connect"

	chatv1 "github.com/reno/go-termchat/gen/go/chat/v1"
	"github.com/reno/go-termchat/gen/go/chat/v1/chatv1connect"
	"github.com/reno/go-termchat/internal/session"
)

type SessionHandler struct {
	sessions *session.Manager
}

var _ chatv1connect.SessionServiceHandler = (*SessionHandler)(nil)

func NewSessionHandler(sessions *session.Manager) *SessionHandler {
	return &SessionHandler{sessions: sessions}
}

func (h *SessionHandler) Join(
	ctx context.Context,
	req *connect.Request[chatv1.JoinRequest],
) (*connect.Response[chatv1.JoinResponse], error) {
	s, token, err := h.sessions.Join(ctx, req.Msg.GetNick())
	if err != nil {
		return nil, Error(err)
	}

	res := connect.NewResponse(&chatv1.JoinResponse{Session: SessionProto(s)})
	session.Attach(res.Header(), h.sessions.Cookies().Issue(token, s.ExpiresAt))
	return res, nil
}

func (h *SessionHandler) SetNick(
	ctx context.Context,
	req *connect.Request[chatv1.SetNickRequest],
) (*connect.Response[chatv1.SetNickResponse], error) {
	current, err := caller(ctx)
	if err != nil {
		return nil, err
	}

	s, err := h.sessions.SetNick(ctx, current.ID, req.Msg.GetNick())
	if err != nil {
		return nil, Error(err)
	}
	return connect.NewResponse(&chatv1.SetNickResponse{Session: SessionProto(s)}), nil
}

func (h *SessionHandler) GetSession(
	ctx context.Context,
	_ *connect.Request[chatv1.GetSessionRequest],
) (*connect.Response[chatv1.GetSessionResponse], error) {
	current, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&chatv1.GetSessionResponse{
		Session: SessionProto(current),
	}), nil
}

func (h *SessionHandler) Leave(
	ctx context.Context,
	_ *connect.Request[chatv1.LeaveRequest],
) (*connect.Response[chatv1.LeaveResponse], error) {
	current, err := caller(ctx)
	if err != nil {
		return nil, err
	}

	if err := h.sessions.Leave(ctx, current.ID); err != nil {
		return nil, Error(err)
	}

	res := connect.NewResponse(&chatv1.LeaveResponse{})
	session.Attach(res.Header(), h.sessions.Cookies().Revoke())
	return res, nil
}
