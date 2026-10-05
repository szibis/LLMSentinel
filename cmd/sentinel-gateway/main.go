package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/szibis/claude-escalate/internal/localgateway"
)

func main() { os.Exit(run()) }

func run() int {
	listen := flag.String("listen", "127.0.0.1:19090", "Loopback lab listener")
	upstream := flag.String("upstream", "http://127.0.0.1:19091/v1", "Separate local MLX-Flash endpoint")
	haiku := flag.String("role-haiku-upstream", "", "Small local MLX-Flash endpoint for Claude Haiku")
	sonnet := flag.String("role-sonnet-upstream", "", "Large local MLX-Flash endpoint for Claude Sonnet")
	opus := flag.String("role-opus-upstream", "", "Large local MLX-Flash endpoint with thinking for Claude Opus")
	timeout := flag.Duration("timeout", 5*time.Minute, "Maximum inference request duration (at most 10m)")
	claude := flag.Bool("claude-adapter", true, "Experimental Claude Messages and validated JSON tool bridge")
	claudeTokens := flag.Int("claude-max-tokens", 768, "Maximum output tokens per local Claude generation (1..32768)")
	bufferedValidation := flag.Bool("claude-buffered-validation", true, "Validate buffered model output before opening the Claude stream")
	flag.Parse()
	host, _, err := net.SplitHostPort(*listen)
	ip := net.ParseIP(host)
	if err != nil || ip == nil || !ip.IsLoopback() {
		fmt.Fprintln(os.Stderr, "listener must use a literal loopback IP and port")
		return 2
	}
	cfg := localgateway.Config{Upstream: *upstream, Timeout: *timeout, MaxRequestBytes: 2 * 1024 * 1024, ClaudeAdapter: *claude, ClaudeMaxTokens: *claudeTokens, ClaudeBufferedValidation: *bufferedValidation}
	if *haiku != "" || *sonnet != "" || *opus != "" {
		if *haiku == "" || *sonnet == "" || *opus == "" {
			fmt.Fprintln(os.Stderr, "Configure all three role endpoints together")
			return 2
		}
		cfg.RoleUpstreams = map[string]string{"haiku": *haiku, "sonnet": *sonnet, "opus": *opus}
		cfg.Router = localgateway.RoleRouter{}
	}
	g, err := localgateway.New(cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	defer g.Close()
	listener, err := net.Listen("tcp", *listen)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	server := &http.Server{Handler: g, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 64 * 1024, BaseContext: func(net.Listener) context.Context { return ctx }}
	finished := make(chan error, 1)
	go func() { finished <- server.Serve(listener) }()
	fmt.Fprintf(os.Stderr, "Sentinel local gateway: http://%s → %s\nClaude adapter: %t (experimental JSON tool bridge). Codex Responses pending; Jes bootstrap policy only.\n", listener.Addr(), *upstream, *claude)
	select {
	case err := <-finished:
		if err != http.ErrServerClosed {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdown); err != nil {
			_ = server.Close()
		}
	}
	return 0
}
