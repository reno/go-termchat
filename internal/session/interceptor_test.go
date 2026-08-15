package session_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"connectrpc.com/connect"

	"github.com/reno/go-termchat/internal/chat"
	"github.com/reno/go-termchat/internal/config"
	"github.com/reno/go-termchat/internal/session"
	"github.com/reno/go-termchat/internal/store"
)

const streamProcedure = "/chat.v1.ChatService/StreamEvents"

type fakeStream struct {
	procedure string
	request   http.Header
	response  http.Header
}

func newFakeStream(procedure string) *fakeStream {
	return &fakeStream{
		procedure: procedure,
		request:   http.Header{},
		response:  http.Header{},
	}
}

func (f *fakeStream) Spec() connect.Spec {
	return connect.Spec{Procedure: f.procedure, StreamType: connect.StreamTypeServer}
}
func (f *fakeStream) Peer() connect.Peer           { return connect.Peer{} }
func (f *fakeStream) Receive(any) error            { return nil }
func (f *fakeStream) Send(any) error               { return nil }
func (f *fakeStream) RequestHeader() http.Header   { return f.request }
func (f *fakeStream) ResponseHeader() http.Header  { return f.response }
func (f *fakeStream) ResponseTrailer() http.Header { return http.Header{} }
func (f *fakeStream) Conn() any                    { return nil }

func newManager(t *testing.T) (*session.Manager, session.Cookies) {
	t.Helper()

	memory := store.NewMemory(config.Retention{
		MessagesPerChannel: 8,
		MessageTTL:         time.Hour,
		SweepInterval:      time.Minute,
	})
	cookies := session.NewCookies("")
	manager := session.NewManager(
		memory, newSigner(t), chat.NewRules(chat.DefaultLimits()),
		cookies, 30*time.Minute,
	)
	return manager, cookies
}

func TestStreamingHandlerRejectsAnUnauthenticatedConnection(t *testing.T) {
	t.Parallel()

	manager, _ := newManager(t)
	interceptor := session.NewInterceptor(manager, nil, "/chat.v1.SessionService/Join")

	called := false
	wrapped := interceptor.WrapStreamingHandler(func(context.Context, connect.StreamingHandlerConn) error {
		called = true
		return nil
	})

	err := wrapped(t.Context(), newFakeStream(streamProcedure))
	if err == nil {
		t.Fatal("stream accepted without a cookie")
	}
	if connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Errorf("code = %s, want unauthenticated", connect.CodeOf(err))
	}
	if called {
		t.Error("the handler ran despite the rejection")
	}
}

func TestStreamingHandlerResolvesTheSessionAndRefreshesTheCookie(t *testing.T) {
	t.Parallel()

	manager, cookies := newManager(t)
	interceptor := session.NewInterceptor(manager, nil, "/chat.v1.SessionService/Join")

	joined, token, err := manager.Join(t.Context(), "rill")
	if err != nil {
		t.Fatalf("Join: %v", err)
	}

	stream := newFakeStream(streamProcedure)
	stream.request.Set("Cookie", cookies.Name()+"="+token)

	var seen chat.Session
	wrapped := interceptor.WrapStreamingHandler(func(ctx context.Context, _ connect.StreamingHandlerConn) error {
		seen, _ = session.From(ctx)
		return nil
	})

	if err := wrapped(t.Context(), stream); err != nil {
		t.Fatalf("stream rejected with a valid cookie: %v", err)
	}
	if seen.ID != joined.ID {
		t.Errorf("session in context = %q, want %q", seen.ID, joined.ID)
	}
	if seen.Nick != "rill" {
		t.Errorf("nick in context = %q", seen.Nick)
	}
	if !cookies.Present(stream.response) {
		t.Error("the stream did not refresh the sliding cookie")
	}
}

func TestStreamingHandlerHonoursTheAnonymousAllowList(t *testing.T) {
	t.Parallel()

	manager, _ := newManager(t)
	interceptor := session.NewInterceptor(manager, nil, streamProcedure)

	called := false
	wrapped := interceptor.WrapStreamingHandler(func(ctx context.Context, _ connect.StreamingHandlerConn) error {
		called = true
		if _, ok := session.From(ctx); ok {
			t.Error("an allow-listed procedure should carry no session")
		}
		return nil
	})

	if err := wrapped(t.Context(), newFakeStream(streamProcedure)); err != nil {
		t.Fatalf("allow-listed stream rejected: %v", err)
	}
	if !called {
		t.Error("the handler never ran")
	}
}

func TestStreamingHandlerRejectsARevokedSession(t *testing.T) {
	t.Parallel()

	manager, cookies := newManager(t)
	interceptor := session.NewInterceptor(manager, nil, "/chat.v1.SessionService/Join")

	joined, token, err := manager.Join(t.Context(), "rill")
	if err != nil {
		t.Fatalf("Join: %v", err)
	}
	if err := manager.Leave(t.Context(), joined.ID); err != nil {
		t.Fatalf("Leave: %v", err)
	}

	stream := newFakeStream(streamProcedure)
	stream.request.Set("Cookie", cookies.Name()+"="+token)

	wrapped := interceptor.WrapStreamingHandler(func(context.Context, connect.StreamingHandlerConn) error {
		t.Error("the handler ran for a revoked session")
		return nil
	})

	if code := connect.CodeOf(wrapped(t.Context(), stream)); code != connect.CodeUnauthenticated {
		t.Errorf("code = %s, want unauthenticated", code)
	}
}

func TestClientStreamsArePassedThrough(t *testing.T) {
	t.Parallel()

	manager, _ := newManager(t)
	interceptor := session.NewInterceptor(manager, nil)

	called := false
	wrapped := interceptor.WrapStreamingClient(func(context.Context, connect.Spec) connect.StreamingClientConn {
		called = true
		return nil
	})
	wrapped(t.Context(), connect.Spec{Procedure: streamProcedure})

	if !called {
		t.Error("the client stream was not passed through untouched")
	}
}
