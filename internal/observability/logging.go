package observability

import (
	"context"
	"crypto/rand"
	"encoding/base32"
	"io"
	"log/slog"
	"time"

	"connectrpc.com/connect"
)

type ctxKey struct{}

const RequestIDHeader = "X-Request-Id"

func NewLogger(out io.Writer, level slog.Level) *slog.Logger {
	return slog.New(slog.NewJSONHandler(out, &slog.HandlerOptions{Level: level}))
}

func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, ctxKey{}, id)
}

func RequestID(ctx context.Context) string {
	id, ok := ctx.Value(ctxKey{}).(string)
	if !ok {
		return ""
	}
	return id
}

func NewRequestID() string {
	raw := make([]byte, 10)
	if _, err := rand.Read(raw); err != nil {
		return "unknown"
	}
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(raw)
}

func Interceptor(log *slog.Logger, metrics *Metrics) connect.Interceptor {
	return &interceptor{log: log, metrics: metrics}
}

type interceptor struct {
	log     *slog.Logger
	metrics *Metrics
}

func (i *interceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		if req.Spec().IsClient {
			return next(ctx, req)
		}

		id := requestIDOf(req.Header())
		ctx = WithRequestID(ctx, id)

		started := time.Now()
		res, err := next(ctx, req)
		elapsed := time.Since(started)

		i.metrics.Observe(req.Spec().Procedure, err)
		i.record(ctx, req.Spec().Procedure, id, elapsed, err)

		if res != nil {
			res.Header().Set(RequestIDHeader, id)
		}
		return res, err
	}
}

func (i *interceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return next
}

func (i *interceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return func(ctx context.Context, conn connect.StreamingHandlerConn) error {
		id := requestIDOf(conn.RequestHeader())
		ctx = WithRequestID(ctx, id)
		conn.ResponseHeader().Set(RequestIDHeader, id)

		i.metrics.StreamOpened()
		defer i.metrics.StreamClosed()

		started := time.Now()
		err := next(ctx, conn)
		elapsed := time.Since(started)

		i.metrics.Observe(conn.Spec().Procedure, err)
		i.record(ctx, conn.Spec().Procedure, id, elapsed, err)
		return err
	}
}

func (i *interceptor) record(
	ctx context.Context,
	procedure, id string,
	elapsed time.Duration,
	err error,
) {
	attrs := []slog.Attr{
		slog.String("procedure", procedure),
		slog.String("request_id", id),
		slog.Int64("duration_ms", elapsed.Milliseconds()),
		slog.String("code", codeOf(err)),
	}

	level := slog.LevelInfo
	if err != nil {
		level = slog.LevelWarn
	}
	i.log.LogAttrs(ctx, level, "rpc", attrs...)
}

func requestIDOf(header interface{ Get(string) string }) string {
	if id := header.Get(RequestIDHeader); id != "" && len(id) <= 64 {
		return id
	}
	return NewRequestID()
}

func codeOf(err error) string {
	if err == nil {
		return "ok"
	}
	return connect.CodeOf(err).String()
}
