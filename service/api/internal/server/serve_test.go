package server

import (
	"context"
	"net"
	"testing"

	"github.com/labstack/echo/v5"
)

// TestServeReportsListenFailure: a service that cannot listen (the port is taken) must fail, not exit as if it had
// been stopped.
func TestServeReportsListenFailure(t *testing.T) {
	t.Parallel()

	taken, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer taken.Close()

	err = serve(context.Background(), taken.Addr().String(), echo.New())
	if err == nil {
		t.Fatal("serve on a taken address returned nil")
	}
}

// TestServeStopsCleanlyOnCancel: a cancelled context is a normal stop.
func TestServeStopsCleanlyOnCancel(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := serve(ctx, "127.0.0.1:0", echo.New())
	if err != nil {
		t.Fatalf("serve after cancel: %v", err)
	}
}
