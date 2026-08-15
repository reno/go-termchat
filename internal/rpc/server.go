package rpc

import (
	"net/http"

	"connectrpc.com/connect"
	"connectrpc.com/grpcreflect"

	"github.com/reno/go-termchat/gen/go/chat/v1/chatv1connect"
	"github.com/reno/go-termchat/internal/session"
)

type Services struct {
	Sessions *SessionHandler
	Chat     *ChatHandler
	Manager  *session.Manager
}

func NewMux(services Services) *http.ServeMux {
	interceptors := connect.WithInterceptors(session.NewInterceptor(
		services.Manager,
		Error,
		chatv1connect.SessionServiceJoinProcedure,
	))

	mux := http.NewServeMux()
	mux.Handle(chatv1connect.NewSessionServiceHandler(services.Sessions, interceptors))
	mux.Handle(chatv1connect.NewChatServiceHandler(services.Chat, interceptors))

	reflector := grpcreflect.NewStaticReflector(
		chatv1connect.SessionServiceName,
		chatv1connect.ChatServiceName,
	)
	mux.Handle(grpcreflect.NewHandlerV1(reflector))
	mux.Handle(grpcreflect.NewHandlerV1Alpha(reflector))

	return mux
}
