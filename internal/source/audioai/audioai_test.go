package audioai

import (
	"context"
	"encoding/binary"
	"math"
	"net"
	"testing"
	"time"

	"github.com/npc1607/dualsense-c/internal/profile"
	"github.com/npc1607/dualsense-c/internal/signal"
	"github.com/npc1607/dualsense-c/internal/source"
)

func TestDecodeJSONPacket(t *testing.T) {
	jsonPayload := []byte(`{"surface_water": 0.85, "audio_gunshot": 0.92, "unknown_alien_sound": 0.5}`)
	vals, ok := decodePacket(jsonPayload)
	if !ok {
		t.Fatal("decodePacket failed for valid JSON")
	}
	if math.Abs(float64(vals["surface_water"])-0.85) > 1e-5 {
		t.Errorf("surface_water = %v, want 0.85", vals["surface_water"])
	}
	if math.Abs(float64(vals["audio_gunshot"])-0.92) > 1e-5 {
		t.Errorf("audio_gunshot = %v, want 0.92", vals["audio_gunshot"])
	}
	if _, ok := vals["unknown_alien_sound"]; ok {
		t.Error("non-standard signal should be filtered out")
	}
}

func TestDecodeBinaryPacket(t *testing.T) {
	waterID, ok := signal.ID("surface_water")
	if !ok {
		t.Fatal("surface_water id not found")
	}
	gunID, ok := signal.ID("audio_gunshot")
	if !ok {
		t.Fatal("audio_gunshot id not found")
	}

	buf := make([]byte, 8+2*5)
	binary.LittleEndian.PutUint32(buf[0:4], binaryMagic)
	binary.LittleEndian.PutUint16(buf[4:6], 1)
	buf[6] = 2 // 2 items
	buf[7] = 0

	// Item 1: surface_water = 0.75
	buf[8] = waterID
	binary.LittleEndian.PutUint32(buf[9:13], math.Float32bits(0.75))

	// Item 2: audio_gunshot = 0.99
	buf[13] = gunID
	binary.LittleEndian.PutUint32(buf[14:18], math.Float32bits(0.99))

	vals, ok := decodePacket(buf)
	if !ok {
		t.Fatal("decodePacket failed for binary format")
	}
	if math.Abs(float64(vals["surface_water"])-0.75) > 1e-5 {
		t.Errorf("surface_water = %v, want 0.75", vals["surface_water"])
	}
	if math.Abs(float64(vals["audio_gunshot"])-0.99) > 1e-5 {
		t.Errorf("audio_gunshot = %v, want 0.99", vals["audio_gunshot"])
	}
}

func TestRunLifecycle(t *testing.T) {
	// Pick an ephemeral local port
	addr := "127.0.0.1:29998"
	s, err := New(profile.Source{
		Type:   "audio_ai",
		Params: map[string]any{"listen": addr},
	}, source.Deps{})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	out := make(chan signal.Frame, 10)
	errCh := make(chan error, 1)
	go func() {
		errCh <- s.Run(ctx, out)
	}()

	// Wait for listener to bind
	time.Sleep(30 * time.Millisecond)

	// Send one UDP packet
	raddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	conn, err := net.DialUDP("udp", nil, raddr)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	if _, err := conn.Write([]byte(`{"surface_metal": 0.88}`)); err != nil {
		t.Fatalf("write: %v", err)
	}

	select {
	case frame := <-out:
		if frame.Values["surface_metal"] != 0.88 {
			t.Errorf("got surface_metal = %v, want 0.88", frame.Values["surface_metal"])
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timeout waiting for signal frame")
	}

	// Cancel context to stop Run
	cancel()
	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("Run returned error on cancel: %v", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timeout waiting for Run to stop")
	}
}
