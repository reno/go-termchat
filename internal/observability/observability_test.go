package observability_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"connectrpc.com/connect"

	"github.com/reno/go-termchat/internal/observability"
)

type recordedLine map[string]any

func logLines(t *testing.T, raw []byte) []recordedLine {
	t.Helper()

	var lines []recordedLine
	for _, entry := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		if entry == "" {
			continue
		}
		var line recordedLine
		if err := json.Unmarshal([]byte(entry), &line); err != nil {
			t.Fatalf("log line is not JSON: %v", err)
		}
		lines = append(lines, line)
	}
	return lines
}

type fakeRequest struct {
	connect.AnyRequest
	header http.Header
	spec   connect.Spec
}

func (r *fakeRequest) Header() http.Header { return r.header }
func (r *fakeRequest) Spec() connect.Spec  { return r.spec }

type fakeResponse struct {
	connect.AnyResponse
	header http.Header
}

func (r *fakeResponse) Header() http.Header { return r.header }

func TestRequestIDRoundTripsThroughContext(t *testing.T) {
	t.Parallel()

	ctx := observability.WithRequestID(t.Context(), "abc")
	if got := observability.RequestID(ctx); got != "abc" {
		t.Errorf("RequestID = %q", got)
	}
	if got := observability.RequestID(t.Context()); got != "" {
		t.Errorf("RequestID on a bare context = %q, want empty", got)
	}
}

func TestNewRequestIDIsDistinct(t *testing.T) {
	t.Parallel()

	seen := make(map[string]struct{}, 500)
	for range 500 {
		id := observability.NewRequestID()
		if id == "" || id == "unknown" {
			t.Fatalf("NewRequestID returned %q", id)
		}
		if _, dup := seen[id]; dup {
			t.Fatalf("NewRequestID repeated %q", id)
		}
		seen[id] = struct{}{}
	}
}

func TestUnaryLoggingRecordsTheCallWithoutSecrets(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	metrics := observability.NewMetrics()
	interceptor := observability.Interceptor(observability.NewLogger(&out, slog.LevelInfo), metrics)

	req := &fakeRequest{
		header: http.Header{},
		spec:   connect.Spec{Procedure: "/chat.v1.ChatService/SendMessage"},
	}
	res := &fakeResponse{header: http.Header{}}

	wrapped := interceptor.WrapUnary(func(context.Context, connect.AnyRequest) (connect.AnyResponse, error) {
		return res, nil
	})
	if _, err := wrapped(t.Context(), req); err != nil {
		t.Fatalf("wrapped call: %v", err)
	}

	lines := logLines(t, out.Bytes())
	if len(lines) != 1 {
		t.Fatalf("got %d log lines, want 1", len(lines))
	}

	line := lines[0]
	if line["procedure"] != "/chat.v1.ChatService/SendMessage" {
		t.Errorf("procedure = %v", line["procedure"])
	}
	if line["code"] != "ok" {
		t.Errorf("code = %v", line["code"])
	}
	if line["request_id"] == "" || line["request_id"] == nil {
		t.Error("no request_id on the log line")
	}
	if _, has := line["duration_ms"]; !has {
		t.Error("no duration_ms on the log line")
	}

	for _, forbidden := range []string{"nick", "session_id", "body", "message"} {
		if _, present := line[forbidden]; present {
			t.Errorf("log line carries a forbidden key %q", forbidden)
		}
	}
	if res.header.Get(observability.RequestIDHeader) == "" {
		t.Error("the response does not echo the request id")
	}
}

func TestLoggingHonoursAnInboundRequestID(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	interceptor := observability.Interceptor(
		observability.NewLogger(&out, slog.LevelInfo),
		observability.NewMetrics(),
	)

	header := http.Header{}
	header.Set(observability.RequestIDHeader, "from-the-edge")

	wrapped := interceptor.WrapUnary(func(context.Context, connect.AnyRequest) (connect.AnyResponse, error) {
		return &fakeResponse{header: http.Header{}}, nil
	})
	if _, err := wrapped(t.Context(), &fakeRequest{header: header, spec: connect.Spec{Procedure: "/p"}}); err != nil {
		t.Fatalf("wrapped call: %v", err)
	}

	if got := logLines(t, out.Bytes())[0]["request_id"]; got != "from-the-edge" {
		t.Errorf("request_id = %v, want the inbound one", got)
	}
}

func TestLoggingRejectsAnOversizedInboundRequestID(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	interceptor := observability.Interceptor(
		observability.NewLogger(&out, slog.LevelInfo),
		observability.NewMetrics(),
	)

	header := http.Header{}
	header.Set(observability.RequestIDHeader, strings.Repeat("x", 200))

	wrapped := interceptor.WrapUnary(func(context.Context, connect.AnyRequest) (connect.AnyResponse, error) {
		return &fakeResponse{header: http.Header{}}, nil
	})
	if _, err := wrapped(t.Context(), &fakeRequest{header: header, spec: connect.Spec{Procedure: "/p"}}); err != nil {
		t.Fatalf("wrapped call: %v", err)
	}

	got, ok := logLines(t, out.Bytes())[0]["request_id"].(string)
	if !ok {
		t.Fatal("request_id is not a string")
	}
	if len(got) > 64 {
		t.Errorf("a caller-supplied id of %d chars reached the log", len(got))
	}
}

func TestFailedCallsLogTheirCode(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	metrics := observability.NewMetrics()
	interceptor := observability.Interceptor(observability.NewLogger(&out, slog.LevelInfo), metrics)

	wrapped := interceptor.WrapUnary(func(context.Context, connect.AnyRequest) (connect.AnyResponse, error) {
		return nil, connect.NewError(connect.CodeAlreadyExists, errors.New("nick already taken"))
	})
	_, err := wrapped(t.Context(), &fakeRequest{
		header: http.Header{},
		spec:   connect.Spec{Procedure: "/chat.v1.SessionService/Join"},
	})
	if err == nil {
		t.Fatal("expected the error to pass through")
	}

	line := logLines(t, out.Bytes())[0]
	if line["code"] != "already_exists" {
		t.Errorf("code = %v", line["code"])
	}
	if line["level"] != "WARN" {
		t.Errorf("level = %v, want WARN", line["level"])
	}

	snapshot := metrics.Snapshot()
	if snapshot.Calls["/chat.v1.SessionService/Join"] != 1 {
		t.Errorf("calls = %v", snapshot.Calls)
	}
	if snapshot.Failures["/chat.v1.SessionService/Join already_exists"] != 1 {
		t.Errorf("failures = %v", snapshot.Failures)
	}
}

func TestMetricsTrackOpenStreams(t *testing.T) {
	t.Parallel()

	metrics := observability.NewMetrics()

	metrics.StreamOpened()
	metrics.StreamOpened()
	if snapshot := metrics.Snapshot(); snapshot.OpenStreams != 2 || snapshot.PeakStreams != 2 {
		t.Errorf("open = %d peak = %d", snapshot.OpenStreams, snapshot.PeakStreams)
	}

	metrics.StreamClosed()
	snapshot := metrics.Snapshot()
	if snapshot.OpenStreams != 1 {
		t.Errorf("open = %d, want 1", snapshot.OpenStreams)
	}
	if snapshot.PeakStreams != 2 {
		t.Errorf("peak = %d, want the high-water mark of 2", snapshot.PeakStreams)
	}
}

func TestMetricsAreRaceFree(t *testing.T) {
	t.Parallel()

	metrics := observability.NewMetrics()

	var wg sync.WaitGroup
	wg.Add(20)
	for range 20 {
		go func() {
			defer wg.Done()
			for range 50 {
				metrics.Observe("/p", nil)
				metrics.StreamOpened()
				metrics.StreamClosed()
				metrics.Snapshot()
			}
		}()
	}
	wg.Wait()

	if got := metrics.Snapshot().Calls["/p"]; got != 1000 {
		t.Errorf("calls = %d, want 1000", got)
	}
}

func TestHealthAndReady(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	draining := false
	observability.Register(mux, func() bool { return !draining }, observability.NewMetrics())

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	get := func(path string) int {
		t.Helper()

		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, server.URL+path, http.NoBody)
		if err != nil {
			t.Fatalf("request: %v", err)
		}
		res, err := server.Client().Do(req)
		if err != nil {
			t.Fatalf("get %s: %v", path, err)
		}
		defer func() { _ = res.Body.Close() }()
		return res.StatusCode
	}

	if code := get(observability.HealthPath); code != http.StatusOK {
		t.Errorf("healthz = %d", code)
	}
	if code := get(observability.ReadyPath); code != http.StatusOK {
		t.Errorf("readyz = %d", code)
	}

	draining = true
	if code := get(observability.ReadyPath); code != http.StatusServiceUnavailable {
		t.Errorf("readyz while draining = %d, want 503", code)
	}
	if code := get(observability.HealthPath); code != http.StatusOK {
		t.Errorf("healthz while draining = %d, want 200: the process is alive, just not taking work", code)
	}
}

func TestMetricsEndpointCarriesNoIdentities(t *testing.T) {
	t.Parallel()

	metrics := observability.NewMetrics()
	metrics.Observe("/chat.v1.ChatService/SendMessage", nil)

	mux := http.NewServeMux()
	observability.Register(mux, func() bool { return true }, metrics)

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, server.URL+observability.MetricsPath, http.NoBody)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	res, err := server.Client().Do(req)
	if err != nil {
		t.Fatalf("get metrics: %v", err)
	}
	defer func() { _ = res.Body.Close() }()

	var payload map[string]any
	if decodeErr := json.NewDecoder(res.Body).Decode(&payload); decodeErr != nil {
		t.Fatalf("decode: %v", decodeErr)
	}

	body, marshalErr := json.Marshal(payload)
	if marshalErr != nil {
		t.Fatalf("marshal: %v", marshalErr)
	}
	for _, forbidden := range []string{"nick", "session", "rill"} {
		if strings.Contains(strings.ToLower(string(body)), forbidden) {
			t.Errorf("metrics payload mentions %q: %s", forbidden, body)
		}
	}
}
