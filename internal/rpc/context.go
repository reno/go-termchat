package rpc

import (
	"context"
	"errors"

	"connectrpc.com/connect"

	"github.com/reno/go-termchat/internal/chat"
	"github.com/reno/go-termchat/internal/session"
)

var errNoSession = errors.New("no session")

func caller(ctx context.Context) (chat.Session, error) {
	s, ok := session.From(ctx)
	if !ok {
		return chat.Session{}, connect.NewError(connect.CodeUnauthenticated, errNoSession)
	}
	return s, nil
}
