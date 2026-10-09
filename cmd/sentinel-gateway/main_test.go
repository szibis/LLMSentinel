package main

import (
	"context"
	"errors"
	"flag"
	"io"
	"net"
	"os"
	"strings"
	"sync"
	"testing"
)

type memoryListener struct {
	closed chan struct{}
	once   sync.Once
	err    error
}

func (l *memoryListener) Accept() (net.Conn, error) {
	if l.err != nil {
		return nil, l.err
	}
	<-l.closed
	return nil, net.ErrClosed
}
func (l *memoryListener) Close() error { l.once.Do(func() { close(l.closed) }); return nil }
func (l *memoryListener) Addr() net.Addr {
	return &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 19090}
}
func runGatewayTest(t *testing.T, args []string) (int, string) {
	t.Helper()
	oldArgs, oldFlags, oldErr := os.Args, flag.CommandLine, os.Stderr
	os.Args = append([]string{"sentinel-gateway"}, args...)
	flag.CommandLine = flag.NewFlagSet("test", flag.ContinueOnError)
	f, e := os.CreateTemp(t.TempDir(), "stderr")
	if e != nil {
		t.Fatal(e)
	}
	os.Stderr = f
	defer func() { os.Args = oldArgs; flag.CommandLine = oldFlags; os.Stderr = oldErr; f.Close() }()
	code := run()
	f.Seek(0, 0)
	b, _ := io.ReadAll(f)
	return code, string(b)
}
func TestGatewayRejectsInvalidConfiguration(t *testing.T) {
	tests := [][]string{{"--listen", "localhost:10"}, {"--listen", "0.0.0.0:10"}, {"--mode", "unknown"}, {"--role-haiku-upstream", "http://127.0.0.1:10001/v1"}, {"--upstream", "https://example.com/v1"}, {"--mode", "learning"}}
	for _, args := range tests {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			code, out := runGatewayTest(t, args)
			if code != 2 || out == "" {
				t.Fatal(code, out)
			}
		})
	}
}
func TestGatewayListenerFailure(t *testing.T) {
	old := listenGateway
	listenGateway = func(network, address string) (net.Listener, error) {
		if network != "tcp" || address != "127.0.0.1:19090" {
			t.Fatal(network, address)
		}
		return nil, errors.New("fixture bind failure")
	}
	defer func() { listenGateway = old }()
	code, out := runGatewayTest(t, nil)
	if code != 1 || !strings.Contains(out, "fixture bind failure") {
		t.Fatal(code, out)
	}
}
func TestGatewayServeAndShutdown(t *testing.T) {
	for _, failure := range []bool{false, true} {
		t.Run(map[bool]string{true: "serve failure", false: "cancel shutdown"}[failure], func(t *testing.T) {
			oldListen, oldContext := listenGateway, gatewaySignalContext
			defer func() { listenGateway = oldListen; gatewaySignalContext = oldContext }()
			listener := &memoryListener{closed: make(chan struct{})}
			if failure {
				listener.err = errors.New("fixture accept failure")
			}
			listenGateway = func(string, string) (net.Listener, error) { return listener, nil }
			gatewaySignalContext = func(parent context.Context, _ ...os.Signal) (context.Context, context.CancelFunc) {
				ctx, cancel := context.WithCancel(parent)
				if !failure {
					cancel()
				}
				return ctx, cancel
			}
			dir := t.TempDir()
			os.Chmod(dir, 0700)
			args := []string{"--role-haiku-upstream", "http://127.0.0.1:10001/v1", "--role-sonnet-upstream", "http://127.0.0.1:10002/v1", "--role-opus-upstream", "http://127.0.0.1:10003/v1", "--training-mode", "--training-dir", dir}
			code, out := runGatewayTest(t, args)
			want := 0
			if failure {
				want = 1
			}
			if code != want || !strings.Contains(out, "Sentinel gateway") {
				t.Fatal(code, out)
			}
		})
	}
}
