package rpc

import (
	"net/http"

	"connectrpc.com/connect"
	"connectrpc.com/grpcreflect"

	"github.com/reno/go-termchat/gen/go/chat/v1/chatv1connect"
	"github.com/reno/go-termchat/internal/session"
)

const MaxRequestBytes = 64 << 10

type Services struct {
	Sessions  *SessionHandler
	Chat      *ChatHandler
	Manager   *session.Manager
	Telemetry connect.Interceptor
}

func NewMux(services Services) *http.ServeMux {
	authenticate := session.NewInterceptor(
		services.Manager,
		Error,
		chatv1connect.SessionServiceJoinProcedure,
	)

	chain := []connect.Interceptor{authenticate}
	if services.Telemetry != nil {
		chain = []connect.Interceptor{services.Telemetry, authenticate}
	}

	options := []connect.HandlerOption{
		connect.WithInterceptors(chain...),
		connect.WithReadMaxBytes(MaxRequestBytes),
	}

	mux := http.NewServeMux()
	mux.Handle(chatv1connect.NewSessionServiceHandler(services.Sessions, options...))
	mux.Handle(chatv1connect.NewChatServiceHandler(services.Chat, options...))

	reflector := grpcreflect.NewStaticReflector(
		chatv1connect.SessionServiceName,
		chatv1connect.ChatServiceName,
	)
	mux.Handle(grpcreflect.NewHandlerV1(reflector))
	mux.Handle(grpcreflect.NewHandlerV1Alpha(reflector))

	return mux
}
