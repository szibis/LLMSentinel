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
	claude := flag.Bool("claude-adapter", true, "Anthropic Messages, OpenAI Responses and Chat Completions with validated local tools")
	claudeTokens := flag.Int("claude-max-tokens", 768, "Maximum output tokens per local Claude generation (1..32768)")
	bufferedValidation := flag.Bool("claude-buffered-validation", true, "Validate buffered model output before opening the Claude stream")
	mode := flag.String("mode", "serving", "serving for local inference; learning for copied hook events; hybrid for explicitly authorized vendor APIs")
	trainingMode := flag.Bool("training-mode", false, "Opt in to private local inference capture")
	trainingDir := flag.String("training-dir", "", "Private directory for training candidates")
	trainingMaxBytes := flag.Int64("training-max-bytes", 32*1024*1024, "Maximum bytes per capture file (one previous file retained)")
	allowPaid := flag.Bool("allow-paid-api", false, "Explicitly allow separately billed commercial APIs in hybrid mode")
	anthropicURL := flag.String("anthropic-base-url", "", "Explicit HTTPS Anthropic API base URL")
	anthropicModel := flag.String("anthropic-model", "", "Commercial model for hybrid Anthropic requests")
	anthropicKeyEnv := flag.String("anthropic-api-key-env", "", "Environment variable name containing hybrid API credentials")
	openaiURL := flag.String("openai-base-url", "", "Explicit HTTPS OpenAI API base URL")
	openaiModel := flag.String("openai-model", "", "Commercial model for hybrid OpenAI requests")
	openaiKeyEnv := flag.String("openai-api-key-env", "", "Environment variable name containing hybrid API credentials")
	flag.Parse()
	host, _, err := net.SplitHostPort(*listen)
	ip := net.ParseIP(host)
	if err != nil || ip == nil || !ip.IsLoopback() {
		fmt.Fprintln(os.Stderr, "listener must use a literal loopback IP and port")
		return 2
	}
	cfg := localgateway.Config{Upstream: *upstream, Timeout: *timeout, MaxRequestBytes: 2 * 1024 * 1024, ClaudeAdapter: *claude, ClaudeMaxTokens: *claudeTokens, ClaudeBufferedValidation: *bufferedValidation}
	if *mode != "serving" && *mode != "learning" && *mode != "hybrid" {
		fmt.Fprintln(os.Stderr, "mode must be serving, learning or hybrid")
		return 2
	}
	cfg.LearningOnly = *mode == "learning"
	if *mode == "hybrid" {
		cfg.Hybrid = &localgateway.HybridConfig{AnthropicBaseURL: *anthropicURL, AnthropicModel: *anthropicModel, AnthropicAPIKeyEnv: *anthropicKeyEnv, OpenAIBaseURL: *openaiURL, OpenAIModel: *openaiModel, OpenAIAPIKeyEnv: *openaiKeyEnv, AllowPaidAPI: *allowPaid}
	}
	if *trainingMode || cfg.LearningOnly {
		cfg.Training = &localgateway.TrainingConfig{Directory: *trainingDir, MaxBytes: *trainingMaxBytes}
	}
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
	fmt.Fprintf(os.Stderr, "Sentinel gateway: http://%s (mode: %s)\nLocal upstream: %s; client adapters: %t (Messages, Responses, Chat Completions). Jes bootstrap policy only.\n", listener.Addr(), *mode, *upstream, *claude)
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
