package session

import (
	"context"

	"connectrpc.com/connect"

	"github.com/reno/go-termchat/internal/chat"
)

type ctxKey struct{}

func NewContext(ctx context.Context, s chat.Session) context.Context {
	return context.WithValue(ctx, ctxKey{}, s)
}

func From(ctx context.Context) (chat.Session, bool) {
	s, ok := ctx.Value(ctxKey{}).(chat.Session)
	return s, ok
}

type Translator func(error) error

func defaultTranslator(err error) error {
	return connect.NewError(connect.CodeUnauthenticated, err)
}

type interceptor struct {
	manager   *Manager
	translate Translator
	anonymous map[string]struct{}
}

func NewInterceptor(m *Manager, translate Translator, allowAnonymous ...string) connect.Interceptor {
	if translate == nil {
		translate = defaultTranslator
	}
	anonymous := make(map[string]struct{}, len(allowAnonymous))
	for _, procedure := range allowAnonymous {
		anonymous[procedure] = struct{}{}
	}
	return &interceptor{manager: m, translate: translate, anonymous: anonymous}
}

func (i *interceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		if req.Spec().IsClient || i.allowed(req.Spec().Procedure) {
			return next(ctx, req)
		}

		token := i.manager.Cookies().Token(req.Header())
		s, err := i.manager.Resolve(ctx, token)
		if err != nil {
			return nil, i.translate(err)
		}

		res, err := next(NewContext(ctx, s), req)
		if err != nil {
			return nil, err
		}
		if !i.manager.Cookies().Present(res.Header()) {
			Attach(res.Header(), i.manager.Cookies().Issue(token, i.manager.Expiry()))
		}
		return res, nil
	}
}

func (i *interceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return next
}

func (i *interceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return func(ctx context.Context, conn connect.StreamingHandlerConn) error {
		if i.allowed(conn.Spec().Procedure) {
			return next(ctx, conn)
		}

		token := i.manager.Cookies().Token(conn.RequestHeader())
		s, err := i.manager.Resolve(ctx, token)
		if err != nil {
			return i.translate(err)
		}

		Attach(conn.ResponseHeader(), i.manager.Cookies().Issue(token, i.manager.Expiry()))
		return next(NewContext(ctx, s), conn)
	}
}

func (i *interceptor) allowed(procedure string) bool {
	_, ok := i.anonymous[procedure]
	return ok
}
