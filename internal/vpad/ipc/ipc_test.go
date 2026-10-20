package ipc

import (
	"context"
	"log"
	"testing"
	"time"

	"github.com/npc1607/dualsense-c/internal/vpad"
)

type discardLogger struct{}

func (discardLogger) Write(p []byte) (int, error) { return len(p), nil }

func TestIPCServerAndClient(t *testing.T) {
	logger := log.New(discardLogger{}, "", 0)
	srv, err := Listen("127.0.0.1:0", logger)
	if err != nil {
		t.Fatalf("Listen failed: %v", err)
	}
	defer srv.Close()

	addr := srv.Endpoint()
	if srv.Subscribers() != 0 {
		t.Errorf("initial subscribers = %d, want 0", srv.Subscribers())
	}

	// Broadcast with 0 clients should return immediately without blocking or error
	srv.Broadcast(vpad.Rumble{Left: 100, Right: 200, At: time.Now()})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	conn, err := Dial(ctx, addr)
	if err != nil {
		t.Fatalf("Dial failed: %v", err)
	}
	defer conn.Close()

	// Wait for server to accept
	time.Sleep(50 * time.Millisecond)
	if srv.Subscribers() != 1 {
		t.Errorf("subscribers = %d, want 1", srv.Subscribers())
	}

	// Test immediate event delivery
	start := time.Now()
	srv.Broadcast(vpad.Rumble{Left: 128, Right: 255, At: time.Now()})

	buf := make([]byte, FrameSize)
	left, right, seq, err := ReadFrame(conn, buf)
	if err != nil {
		t.Fatalf("ReadFrame failed: %v", err)
	}
	elapsed := time.Since(start)
	t.Logf("ReadFrame received in %v (seq=%d)", elapsed, seq)

	if elapsed > 100*time.Millisecond {
		t.Errorf("latency too high: %v", elapsed)
	}
	if left < 0.50 || left > 0.51 {
		t.Errorf("left = %v, want ~0.5019 (128/255)", left)
	}
	if right != 1.0 {
		t.Errorf("right = %v, want 1.0", right)
	}
	if seq == 0 {
		t.Errorf("seq should be > 0, got %d", seq)
	}
}

func TestParseEndpoint(t *testing.T) {
	tests := []struct {
		in      string
		wantNet string
		wantAdr string
	}{
		{"", "tcp", DefaultEndpoint},
		{"127.0.0.1:9998", "tcp", "127.0.0.1:9998"},
		{":9998", "tcp", ":9998"},
		{"tcp://127.0.0.1:9998", "tcp", "127.0.0.1:9998"},
		{"unix:///tmp/ds5vpad.sock", "unix", "/tmp/ds5vpad.sock"},
		{"/var/run/ds5vpad.sock", "unix", "/var/run/ds5vpad.sock"},
		{"@abstract.sock", "unix", "@abstract.sock"},
	}
	for _, tc := range tests {
		netw, addr := ParseEndpoint(tc.in)
		if netw != tc.wantNet || addr != tc.wantAdr {
			t.Errorf("ParseEndpoint(%q) = (%q, %q), want (%q, %q)",
				tc.in, netw, addr, tc.wantNet, tc.wantAdr)
		}
	}
}
