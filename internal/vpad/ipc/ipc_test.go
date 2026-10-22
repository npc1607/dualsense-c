package ipc

import (
	"bytes"
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

func TestSignalFrameEncodeDecode(t *testing.T) {
	signals := map[string]float32{
		"throttle": 0.85,
		"speed":    120.5,
		"rpm":      6500.0,
		"slip":     0.15,
	}
	seq := uint32(42)

	buf := EncodeSignalFrame(seq, signals)
	if len(buf) != SigHeaderSize+4*SigItemSize {
		t.Fatalf("unexpected frame size: got %d, want %d", len(buf), SigHeaderSize+4*SigItemSize)
	}

	decSeq, decSignals, ok := DecodeSignalFrame(buf)
	if !ok {
		t.Fatalf("DecodeSignalFrame failed")
	}
	if decSeq != seq {
		t.Errorf("got seq=%d, want %d", decSeq, seq)
	}
	for k, want := range signals {
		got, exists := decSignals[k]
		if !exists {
			t.Errorf("missing signal %q in decoded output", k)
			continue
		}
		if diff := got - want; diff > 1e-4 || diff < -1e-4 {
			t.Errorf("signal %s: got %f, want %f", k, got, want)
		}
	}

	// Test edge cases: empty signals
	emptyBuf := EncodeSignalFrame(100, nil)
	eSeq, eSignals, ok := DecodeSignalFrame(emptyBuf)
	if !ok || eSeq != 100 || len(eSignals) != 0 {
		t.Errorf("empty frame decode failed: ok=%v, seq=%d, len=%d", ok, eSeq, len(eSignals))
	}

	// Test edge cases: corrupt magic
	corruptMagic := make([]byte, len(buf))
	copy(corruptMagic, buf)
	corruptMagic[0] = 0x00
	if _, _, ok := DecodeSignalFrame(corruptMagic); ok {
		t.Errorf("expected corrupt magic to fail decode")
	}

	// Test edge cases: truncated payload
	if _, _, ok := DecodeSignalFrame(buf[:len(buf)-2]); ok {
		t.Errorf("expected truncated frame to fail decode")
	}
}

func TestServerBroadcastSignals(t *testing.T) {
	logger := log.New(discardLogger{}, "", 0)
	srv, err := Listen("127.0.0.1:0", logger)
	if err != nil {
		t.Fatalf("Listen failed: %v", err)
	}
	defer srv.Close()

	// Zero idle overhead: broadcast with 0 clients must not block
	srv.BroadcastSignals(map[string]float32{"speed": 100})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	conn, err := Dial(ctx, srv.Endpoint())
	if err != nil {
		t.Fatalf("Dial failed: %v", err)
	}
	defer conn.Close()

	time.Sleep(50 * time.Millisecond)

	// Broadcast frame 1
	send1 := map[string]float32{"speed": 120.0, "throttle": 0.95}
	srv.BroadcastSignals(send1)

	seq1, sigs1, err := ReadSignalFrame(conn)
	if err != nil {
		t.Fatalf("ReadSignalFrame 1 failed: %v", err)
	}
	if seq1 != 1 {
		t.Errorf("expected seq 1, got %d", seq1)
	}
	if sigs1["speed"] != 120.0 || sigs1["throttle"] != 0.95 {
		t.Errorf("unexpected signals 1: %v", sigs1)
	}

	// Broadcast frame 2
	send2 := map[string]float32{"brake": 0.5}
	srv.BroadcastSignals(send2)

	seq2, sigs2, err := ReadSignalFrame(conn)
	if err != nil {
		t.Fatalf("ReadSignalFrame 2 failed: %v", err)
	}
	if seq2 != 2 {
		t.Errorf("expected seq 2, got %d", seq2)
	}
	if sigs2["brake"] != 0.5 {
		t.Errorf("unexpected signals 2: %v", sigs2)
	}
}

func TestSpeechFrameEncodeDecode(t *testing.T) {
	frame := SpeechFrame{
		Name:     "critical_alert",
		Target:   "speak",
		Clip:     "low_hp_warning",
		Priority: 90,
		Text:     "Health is dangerously low!",
		At:       time.Now().Truncate(time.Millisecond),
	}

	buf := EncodeSpeechFrame(42, frame)
	decFrame, ok := DecodeSpeechFrame(buf)
	if !ok {
		t.Fatalf("DecodeSpeechFrame failed")
	}
	if decFrame.Seq != 42 || decFrame.Clip != "low_hp_warning" || decFrame.Priority != 90 {
		t.Errorf("unexpected decoded frame: %+v", decFrame)
	}
	if decFrame.Text != frame.Text {
		t.Errorf("text mismatch: got %q, want %q", decFrame.Text, frame.Text)
	}

	// Test ReadSpeechFrame over byte buffer
	r := bytes.NewReader(buf)
	rFrame, err := ReadSpeechFrame(r)
	if err != nil {
		t.Fatalf("ReadSpeechFrame failed: %v", err)
	}
	if rFrame.Seq != 42 || rFrame.Name != "critical_alert" {
		t.Errorf("ReadSpeechFrame mismatch: %+v", rFrame)
	}

	// Corrupt magic
	badBuf := make([]byte, len(buf))
	copy(badBuf, buf)
	badBuf[0] = 0x00
	if _, ok := DecodeSpeechFrame(badBuf); ok {
		t.Errorf("expected corrupt magic to fail decode")
	}
}

func TestServerBroadcast_NoHeadOfLineBlocking(t *testing.T) {
	logger := log.New(discardLogger{}, "", 0)
	srv, err := Listen("127.0.0.1:0", logger)
	if err != nil {
		t.Fatalf("Listen failed: %v", err)
	}
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Connect a client that NEVER reads
	conn, err := Dial(ctx, srv.Endpoint())
	if err != nil {
		t.Fatalf("Dial failed: %v", err)
	}
	defer conn.Close()

	time.Sleep(50 * time.Millisecond)

	// Rapidly push 200 frames (simulating 125Hz input loop)
	start := time.Now()
	for i := 0; i < 200; i++ {
		srv.BroadcastSignals(map[string]float32{"throttle": float32(i)})
		srv.BroadcastSpeech(SpeechFrame{Clip: "test", Priority: 50})
	}
	elapsed := time.Since(start)

	if elapsed > 100*time.Millisecond {
		t.Errorf("broadcasting 200 frames blocked the caller thread: %v", elapsed)
	}
}
