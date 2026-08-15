package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/reno/go-termchat/internal/chat"
	"github.com/reno/go-termchat/internal/config"
	"github.com/reno/go-termchat/internal/hub"
	"github.com/reno/go-termchat/internal/observability"
	"github.com/reno/go-termchat/internal/rpc"
	"github.com/reno/go-termchat/internal/session"
	"github.com/reno/go-termchat/internal/store"
)

func main() {
	log := observability.NewLogger(os.Stdout, slog.LevelInfo)

	if len(os.Args) > 1 && os.Args[1] == "-healthcheck" {
		if err := healthcheck(os.Getenv); err != nil {
			log.Error("healthcheck failed", slog.String("error", err.Error()))
			os.Exit(1)
		}
		return
	}

	if err := run(context.Background(), log, os.Getenv); err != nil {
		log.Error("server stopped", slog.String("error", err.Error()))
		os.Exit(1)
	}
}

func run(ctx context.Context, log *slog.Logger, getenv func(string) string) error {
	cfg, err := config.Load(getenv)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	signer, err := session.NewSigner(cfg.Session.Key)
	if err != nil {
		return err
	}
	if cfg.Session.Generated {
		log.WarnContext(ctx, "session signing key generated for this process, sessions will not survive a restart")
	}

	rules := chat.NewRules(cfg.Limits)
	memory := store.NewMemory(cfg.Retention)
	events := hub.New(cfg.EventBuffer)
	limiter := chat.NewRateLimiter(cfg.Rate)

	manager := session.NewManager(
		memory, signer, rules,
		session.NewCookies(cfg.Session.CookieName),
		cfg.Session.TTL,
	)

	ids := chat.NewIDGen()

	sweeper := store.NewSweeper(memory, cfg.Retention.SweepInterval,
		store.WithSweepHook(func(stats store.SweepStats) {
			limiter.Sweep(time.Now())
			announceDepartures(events, ids, stats.Departed)
			log.InfoContext(ctx, "retention swept",
				slog.Int("messages_dropped", stats.MessagesDropped),
				slog.Int("channels_reaped", stats.ChannelsReaped),
				slog.Int("sessions_expired", stats.SessionsExpired),
			)
		}),
	)

	swept := make(chan error, 1)
	go func() { swept <- sweeper.Run(ctx) }()
	go expireTyping(ctx, memory, events, ids, cfg.TypingTTL)

	metrics := observability.NewMetrics()
	draining := &atomic.Bool{}

	mux := rpc.NewMux(rpc.Services{
		Sessions: rpc.NewSessionHandler(manager),
		Chat: rpc.NewChatHandler(memory, events, rules, limiter, ids, cfg.TypingTTL,
			rpc.WithKeepalive(cfg.Keepalive)),
		Manager:   manager,
		Telemetry: observability.Interceptor(log, metrics),
	})

	observability.Register(mux, func() bool { return !draining.Load() }, metrics)

	if assets, ok := webAssets(); ok {
		mux.Handle("/", rpc.Static(assets))
		log.InfoContext(ctx, "serving the embedded web build")
	}

	handler := rpc.CORS(cfg.CORSOrigins, mux)

	protocols := new(http.Protocols)
	protocols.SetHTTP1(true)
	protocols.SetUnencryptedHTTP2(true)

	server := &http.Server{
		Addr:              cfg.Addr,
		Handler:           handler,
		Protocols:         protocols,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       2 * time.Minute,
		MaxHeaderBytes:    16 << 10,
	}

	listening := make(chan error, 1)
	go func() {
		log.InfoContext(ctx, "listening",
			slog.String("addr", cfg.Addr),
			slog.String("env", string(cfg.Env)),
		)
		listening <- server.ListenAndServe()
	}()

	select {
	case err := <-listening:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case err := <-swept:
		if err != nil && !errors.Is(err, context.Canceled) {
			return err
		}
	case <-ctx.Done():
	}

	shutdown, cancel := context.WithTimeout(context.WithoutCancel(ctx), cfg.ShutdownGrace)
	defer cancel()

	draining.Store(true)
	log.InfoContext(shutdown, "shutting down")
	if err := server.Shutdown(shutdown); err != nil {
		return err
	}
	return nil
}

func announceDepartures(events *hub.Hub, ids *chat.IDGen, departed []store.Departure) {
	now := time.Now()
	for _, departure := range departed {
		for _, channelID := range departure.Channels {
			id, err := ids.New(now)
			if err != nil {
				return
			}
			events.PublishChannel(channelID, chat.MemberLeft(id, now, channelID, departure.Nick))
		}
	}
}

func expireTyping(
	ctx context.Context,
	st store.Store,
	events *hub.Hub,
	ids *chat.IDGen,
	every time.Duration,
) {
	ticker := time.NewTicker(every)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			expired, err := st.ExpireTyping(ctx, now)
			if err != nil {
				return
			}
			for _, entry := range expired {
				id, idErr := ids.New(now)
				if idErr != nil {
					return
				}
				events.PublishChannel(entry.ChannelID,
					chat.TypingChanged(id, now, entry.ChannelID, entry.Nick, false))
			}
		}
	}
}

func healthcheck(getenv func(string) string) error {
	addr := getenv("TERMCHAT_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	if strings.HasPrefix(addr, ":") {
		addr = "127.0.0.1" + addr
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(
		ctx, http.MethodGet, "http://"+addr+observability.ReadyPath, http.NoBody,
	)
	if err != nil {
		return err
	}

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = res.Body.Close() }()

	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("healthcheck: %s returned %d", observability.ReadyPath, res.StatusCode)
	}
	return nil
}
