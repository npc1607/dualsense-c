package ipc_test

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"testing"
	"time"

	"github.com/npc1607/dualsense-c/internal/profile"
	"github.com/npc1607/dualsense-c/internal/rules"
	"github.com/npc1607/dualsense-c/internal/signal"
	"github.com/npc1607/dualsense-c/internal/vpad/ipc"
)

func TestStateFrameEncodeDecode(t *testing.T) {
	hdr := ipc.StateHeader{
		Flags:  1,
		Seq:    42,
		TickMS: 12345,
	}
	sigs := []signal.SignalTelemetry{
		{ID: 0, Name: "throttle", Raw: 0.75, Shaped: 0.70, AgeMS: 5, Fresh: true},
		{ID: 1, Name: "brake", Raw: 0.10, Shaped: 0.05, AgeMS: 10, Fresh: true},
	}
	rls := []rules.RuleTelemetry{
		{RuleID: 10, State: rules.RuleStateTrue, Priority: 50, LastFireMS: 100, UntilMS: 500},
	}
	summary := ipc.StateSummary{
		L2Mode:       2,
		L2Params:     [4]uint8{10, 20, 30, 40},
		R2Mode:       3,
		R2Params:     [4]uint8{50, 60, 70, 80},
		RumbleLeft:   128,
		RumbleRight:  255,
		LightbarR:    255,
		LightbarG:    128,
		LightbarB:    64,
		PlayerLEDs:   4,
		MuteLED:      1,
		RateHz:       125,
		TickJitterUS: 250,
		HIDErrors:    0,
		Subscribers:  1,
		AimMismatch:  0,
	}

	frame := ipc.EncodeStateFrame(hdr, sigs, rls, summary)
	if len(frame) == 0 {
		t.Fatal("encoded frame is empty")
	}

	decodedHdr, decodedSigs, decodedRls, decodedSummary, err := ipc.DecodeStateFrame(frame)
	if err != nil {
		t.Fatalf("DecodeStateFrame failed: %v", err)
	}

	if decodedHdr.Seq != 42 {
		t.Fatalf("expected Seq 42, got %d", decodedHdr.Seq)
	}
	if decodedHdr.TickMS != 12345 {
		t.Fatalf("expected TickMS 12345, got %d", decodedHdr.TickMS)
	}
	if len(decodedSigs) != 2 {
		t.Fatalf("expected 2 sigs, got %d", len(decodedSigs))
	}
	if decodedSigs[0].Name != "throttle" || decodedSigs[0].AgeMS != 5 {
		t.Fatalf("unexpected decoded sig: %+v", decodedSigs[0])
	}
	if len(decodedRls) != 1 || decodedRls[0].RuleID != 10 || decodedRls[0].State != rules.RuleStateTrue {
		t.Fatalf("unexpected decoded rule: %+v", decodedRls[0])
	}
	if decodedSummary.RumbleLeft != 128 || decodedSummary.LightbarR != 255 || decodedSummary.RateHz != 125 {
		t.Fatalf("unexpected decoded summary: %+v", decodedSummary)
	}

	// Test ReadStateFrame from stream
	stream := bytes.NewReader(frame)
	rStateHdr, _, _, _, err := ipc.ReadStateFrame(stream)
	if err != nil {
		t.Fatalf("ReadStateFrame failed: %v", err)
	}
	if rStateHdr.Seq != 42 {
		t.Fatalf("ReadStateFrame Seq expected 42, got %d", rStateHdr.Seq)
	}
}

func TestCommandRPC(t *testing.T) {
	srv, err := ipc.Listen("127.0.0.1:0", nil)
	if err != nil {
		t.Fatalf("listen failed: %v", err)
	}
	defer srv.Close()

	srv.SetCommandHandler(func(cmd ipc.Command) ipc.Response {
		if cmd.Method == "ping" {
			return ipc.Response{
				ID:     cmd.ID,
				OK:     true,
				Result: json.RawMessage(`{"pong":true}`),
			}
		}
		return ipc.Response{
			ID:    cmd.ID,
			OK:    false,
			Error: "unknown method",
		}
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	conn, err := ipc.Dial(ctx, srv.Endpoint())
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}
	defer conn.Close()

	// Send CMD1 request
	cmd := ipc.Command{
		ID:     "req-1",
		Method: "ping",
	}
	if err := ipc.WriteCommand(conn, cmd); err != nil {
		t.Fatalf("WriteCommand failed: %v", err)
	}

	// Read response
	resp, err := ipc.ReadResponse(conn)
	if err != nil {
		t.Fatalf("ReadResponse failed: %v", err)
	}

	if resp.ID != "req-1" {
		t.Fatalf("expected ID req-1, got %q", resp.ID)
	}
	if !resp.OK {
		t.Fatalf("expected OK true, got error: %s", resp.Error)
	}
	if string(resp.Result) != `{"pong":true}` {
		t.Fatalf("unexpected result: %s", string(resp.Result))
	}
}

// TestStateFrameCarriesStableRuleID pins the 13-byte rule item layout while
// checking that the daemon-side telemetry id is the hash-derived stable id,
// not the rule's index in the compiled slice.
func TestStateFrameCarriesStableRuleID(t *testing.T) {
	p := &profile.Profile{
		Name:   "stable",
		Source: profile.Source{Type: "sim"},
		Rules: []profile.Rule{
			{Name: "wall", Target: "rumble_left", Value: "10", Priority: 20},
			{Name: "curb", Target: "rumble_left", Value: "20", Priority: 10},
		},
	}
	eng, err := rules.New(p, signal.NewSet(nil), nil)
	if err != nil {
		t.Fatalf("rules.New: %v", err)
	}
	telem := eng.Telemetry(time.Now())
	if len(telem) != 2 {
		t.Fatalf("expected 2 rules, got %d", len(telem))
	}
	// Priority ascending: curb (10) then wall (20).
	if telem[0].Name != "curb" || telem[1].Name != "wall" {
		t.Fatalf("unexpected telemetry order: %+v", telem)
	}
	if telem[0].RuleID != rules.StableRuleID("curb") || telem[1].RuleID != rules.StableRuleID("wall") {
		t.Fatalf("telemetry ids are not name-derived: %+v", telem)
	}
	frame := ipc.EncodeStateFrame(ipc.StateHeader{Seq: 1}, nil, telem, ipc.StateSummary{})
	wantLen := ipc.StateHeaderSize + 2*ipc.StateRuleItemSize + ipc.StateSummarySize
	if len(frame) != wantLen {
		t.Fatalf("frame length = %d, want %d (13B rule item must not change)", len(frame), wantLen)
	}
	// RuleID is the first field of each 13-byte item.
	for i, want := range []uint16{rules.StableRuleID("curb"), rules.StableRuleID("wall")} {
		off := ipc.StateHeaderSize + i*ipc.StateRuleItemSize
		if got := binary.LittleEndian.Uint16(frame[off : off+2]); got != want {
			t.Fatalf("rule[%d] wire id = %d, want %d", i, got, want)
		}
		// State (1B) + Priority (2B) + LastFireMS (4B) + UntilMS (4B) = 12B follow the id.
		if got := binary.LittleEndian.Uint16(frame[off+3 : off+5]); int16(got) != telem[i].Priority {
			t.Fatalf("rule[%d] priority = %d, want %d", i, int16(got), telem[i].Priority)
		}
	}

	hdr, _, decoded, _, err := ipc.DecodeStateFrame(frame)
	if err != nil {
		t.Fatalf("DecodeStateFrame: %v", err)
	}
	if hdr.RuleCount != 2 {
		t.Fatalf("decoded RuleCount = %d, want 2", hdr.RuleCount)
	}
	for i := range telem {
		if decoded[i].RuleID != telem[i].RuleID {
			t.Fatalf("rule[%d] id changed in roundtrip: %d -> %d", i, telem[i].RuleID, decoded[i].RuleID)
		}
	}
}
