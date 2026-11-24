package main

import (
	"context"
	"errors"
	"io"
	"net"
	"sync/atomic"
	"testing"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

// refusingBroker answers every MQTT connect with CONNACK "not authorised" (return code 5), as the stack's broker does
// with a password file not written from the environment's keys, and counts the attempts.
func refusingBroker(t *testing.T) (string, *atomic.Int32) {
	t.Helper()

	listener, err := new(net.ListenConfig).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = listener.Close() })

	attempts := new(atomic.Int32)

	go func() {
		for {
			conn, acceptErr := listener.Accept()
			if acceptErr != nil {
				return
			}

			attempts.Add(1)

			go func() {
				defer func() { _ = conn.Close() }()

				buf := make([]byte, 256)

				_, readErr := conn.Read(buf)
				if readErr != nil && !errors.Is(readErr, io.EOF) {
					return
				}

				_, _ = conn.Write([]byte{0x20, 0x02, 0x00, 0x05})
			}()
		}
	}()

	return "tcp://" + listener.Addr().String(), attempts
}

// TestConnectTriesAgainAfterARefusal: a refused login is tried again after the retry, not waited on once, and the
// loop ends with the context.
func TestConnectTriesAgainAfterARefusal(t *testing.T) {
	t.Parallel()

	broker, attempts := refusingBroker(t)
	client := mqtt.NewClient(
		mqtt.NewClientOptions().AddBroker(broker).SetClientID("iot-bridge-test"),
	)

	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()

	go func() {
		for attempts.Load() < 2 && ctx.Err() == nil {
			time.Sleep(5 * time.Millisecond)
		}

		cancel()
	}()

	err := connect(ctx, client, broker, 10*time.Millisecond)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("connect = %v, want the context's end", err)
	}

	if got := attempts.Load(); got < 2 {
		t.Fatalf("attempts = %d, want at least 2", got)
	}
}
