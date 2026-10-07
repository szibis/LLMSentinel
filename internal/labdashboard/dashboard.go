// Package labdashboard serves a read-only view of the isolated Sentinel backend.
package labdashboard

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/szibis/claude-escalate/internal/clientcontrol"
	"github.com/szibis/claude-escalate/internal/labstatus"
	"github.com/szibis/claude-escalate/internal/taskquality"
)

//go:embed dashboard.html
var page []byte

func validListen(address string) bool {
	host, port, err := net.SplitHostPort(address)
	n, numberErr := strconv.Atoi(port)
	return err == nil && numberErr == nil && host == "127.0.0.1" && n > 0 && n <= 65535
}

func readEndpoint(client *http.Client, path string) json.RawMessage {
	response, err := client.Get("http://127.0.0.1:19090" + path)
	if err != nil {
		return nil
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, 65537))
	var object map[string]json.RawMessage
	if err != nil || len(raw) > 65536 || json.Unmarshal(raw, &object) != nil || object == nil {
		return nil
	}
	return raw
}

func newHandler(client *http.Client, snapshot func() (json.RawMessage, error)) http.Handler {
	return newHandlerWithQuality(client, snapshot, func() json.RawMessage { return nil })
}

func newHandlerWithQuality(client *http.Client, snapshot func() (json.RawMessage, error), quality func() json.RawMessage) http.Handler {
	var snapshotMu sync.Mutex
	var cachedTelemetry json.RawMessage
	var cachedAt time.Time
	history := newHistory()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		if r.Method != http.MethodGet {
			http.Error(w, "Use GET", http.StatusMethodNotAllowed)
			return
		}
		switch r.URL.Path {
		case "/", "/dashboard":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write(page)
		case "/health":
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"status":"ok","scope":"lab-dashboard"}`)
		case "/api/status":
			var telemetry, gateway, activity json.RawMessage
			var group sync.WaitGroup
			group.Add(3)
			go func() {
				defer group.Done()
				snapshotMu.Lock()
				defer snapshotMu.Unlock()
				if time.Since(cachedAt) >= 4*time.Second {
					cachedTelemetry, _ = snapshot()
					cachedAt = time.Now()
				}
				telemetry = cachedTelemetry
			}()
			go func() { defer group.Done(); gateway = readEndpoint(client, "/health") }()
			go func() { defer group.Done(); activity = readEndpoint(client, "/sentinel/activity") }()
			group.Wait()
			now := time.Now().UTC()
			history.record(telemetry, activity, now)
			points, attempts := history.snapshot(now)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"sample_time": now.Format(time.RFC3339), "telemetry": telemetry, "gateway": gateway, "activity": activity, "history": points, "attempts": attempts, "history_window_seconds": int(historyWindow.Seconds()), "task_quality": quality()})
		default:
			http.NotFound(w, r)
		}
	})
}

// Run starts a loopback-only dashboard. It never sends inference or mutations.
func Run(args []string, _ io.Reader, out, stderr io.Writer) int {
	flags := flag.NewFlagSet("dashboard", flag.ContinueOnError)
	flags.SetOutput(stderr)
	root := flags.String("root", ".sentinel-lab", "isolated lab root")
	listen := flags.String("listen", "127.0.0.1:8077", "loopback listener")
	if flags.Parse(args) != nil || flags.NArg() != 0 {
		return 2
	}
	if !validListen(*listen) {
		fmt.Fprintln(stderr, "dashboard requires 127.0.0.1 and port 1..65535")
		return 2
	}
	absolute, err := filepath.Abs(*root)
	if err == nil {
		err = clientcontrol.ValidateDirectory(absolute)
	}
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	client := &http.Client{Transport: &http.Transport{Proxy: nil}, Timeout: 600 * time.Millisecond, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("redirect refused") }}
	defer client.CloseIdleConnections()
	snapshot := func() (json.RawMessage, error) {
		var data, diagnostic bytes.Buffer
		if labstatus.Run([]string{"--root", absolute, "--json"}, nil, &data, &diagnostic) != 0 {
			return nil, errors.New(diagnostic.String())
		}
		return json.RawMessage(data.Bytes()), nil
	}
	server := &http.Server{Addr: *listen, Handler: newHandlerWithQuality(client, snapshot, func() json.RawMessage {
		report := taskquality.Load(absolute)
		if report == nil {
			return nil
		}
		for i := range report.Results {
			report.Results[i].Exchanges = nil
			report.Results[i].Final = ""
		}
		raw, _ := json.Marshal(report)
		return raw
	}), ReadHeaderTimeout: 3 * time.Second, ReadTimeout: 5 * time.Second, WriteTimeout: 5 * time.Second, IdleTimeout: 30 * time.Second}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdown)
	}()
	fmt.Fprintf(out, "Live Sentinel backend: http://%s/dashboard\n", *listen)
	if err = server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}
