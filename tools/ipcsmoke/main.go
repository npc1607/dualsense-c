// Command ipcsmoke is a headless end-to-end smoke client for ds5d's STATE1 +
// CMD1 control plane. It is meant for development machines without a
// controller:
//
//	./bin/ds5d -dry-run -profile /tmp/stale.yaml &
//	go run ./tools/ipcsmoke
//
// It subscribes to STATE1, checks that the signal count matches
// signal.Standard, looks for at least one STALE rule, then issues health and
// checks that the dry-run device block reports connected:false.
package main

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/npc1607/dualsense-c/internal/signal"
	"github.com/npc1607/dualsense-c/internal/vpad/ipc"
)

func main() {
	endpoint := flag.String("endpoint", ipc.DefaultSignalEndpoint, "ds5d IPC endpoint")
	timeout := flag.Duration("timeout", 5*time.Second, "overall deadline")
	wantStale := flag.Bool("want-stale", true, "require at least one rule in state 3 (STALE)")
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	conn, err := ipc.Dial(ctx, *endpoint)
	if err != nil {
		fail("dial %s: %v", *endpoint, err)
	}
	defer conn.Close()
	deadline, _ := ctx.Deadline()
	_ = conn.SetDeadline(deadline)

	if err := ipc.WriteCommand(conn, ipc.Command{ID: "smoke-sub", Method: "state.subscribe"}); err != nil {
		fail("state.subscribe: %v", err)
	}

	var (
		subOK      bool
		gotState   bool
		sigCount   uint16
		ruleStates []int
		stateSeq   uint32
	)
	for !gotState || !subOK {
		kind, frame, err := readFrame(conn)
		if err != nil {
			fail("read frame: %v", err)
		}
		switch kind {
		case "cmd":
			var resp ipc.Response
			if err := json.Unmarshal(frame, &resp); err != nil {
				fail("decode cmd response: %v", err)
			}
			if resp.ID != "smoke-sub" || !resp.OK {
				fail("state.subscribe failed: ok=%v error=%s", resp.OK, resp.Error)
			}
			subOK = true
		case "state":
			hdr, _, rules, _, err := ipc.DecodeStateFrame(frame)
			if err != nil {
				fail("decode STATE1: %v", err)
			}
			gotState = true
			sigCount = hdr.SignalCount
			stateSeq = hdr.Seq
			ruleStates = ruleStates[:0]
			for _, r := range rules {
				ruleStates = append(ruleStates, int(r.State))
			}
		}
	}

	if err := ipc.WriteCommand(conn, ipc.Command{ID: "smoke-health", Method: "health"}); err != nil {
		fail("health: %v", err)
	}
	var health map[string]any
	for health == nil {
		kind, frame, err := readFrame(conn)
		if err != nil {
			fail("read health response: %v", err)
		}
		if kind != "cmd" {
			continue
		}
		var resp ipc.Response
		if err := json.Unmarshal(frame, &resp); err != nil {
			fail("decode health response: %v", err)
		}
		if resp.ID != "smoke-health" {
			continue
		}
		if !resp.OK {
			fail("health failed: %s", resp.Error)
		}
		if err := json.Unmarshal(resp.Result, &health); err != nil {
			fail("decode health JSON: %v", err)
		}
	}

	dev, _ := health["device"].(map[string]any)
	connected, _ := dev["connected"].(bool)

	stale := false
	for _, st := range ruleStates {
		if st == 3 {
			stale = true
			break
		}
	}

	out := map[string]any{
		"state_seq":       stateSeq,
		"signal_count":    sigCount,
		"want_signals":    len(signal.Standard),
		"rule_states":     ruleStates,
		"stale_rule":      stale,
		"device":          dev,
		"health_uptime_s": health["uptime_s"],
	}
	b, _ := json.MarshalIndent(out, "", "  ")
	fmt.Println(string(b))

	if int(sigCount) != len(signal.Standard) {
		fail("SignalCount = %d, want %d (signal.Standard)", sigCount, len(signal.Standard))
	}
	if *wantStale && !stale {
		fail("no rule reported STALE (state 3): %v", ruleStates)
	}
	if dev == nil {
		fail("health is missing the device object: %v", health)
	}
	if _, exists := dev["connected"]; !exists {
		fail("health device is missing connected: %v", dev)
	}
	if connected {
		fail("dry-run health reported device.connected=true: %v", dev)
	}
	fmt.Println("smoke: ok")
}

type frameKind string

const (
	kindCmd   frameKind = "cmd"
	kindState frameKind = "state"
)

// readFrame demultiplexes the CMD1 and STATE1 frames that share the TCP
// connection. Both start with a little-endian magic; STATE1 then has a fixed
// 20-byte header, CMD1 a 8-byte length prefix.
func readFrame(r io.Reader) (frameKind, []byte, error) {
	var magicBuf [4]byte
	if _, err := io.ReadFull(r, magicBuf[:]); err != nil {
		return "", nil, err
	}
	magic := binary.LittleEndian.Uint32(magicBuf[:])
	switch magic {
	case ipc.MagicCmd:
		var lenBuf [4]byte
		if _, err := io.ReadFull(r, lenBuf[:]); err != nil {
			return "", nil, err
		}
		n := binary.LittleEndian.Uint32(lenBuf[:])
		payload := make([]byte, n)
		if _, err := io.ReadFull(r, payload); err != nil {
			return "", nil, err
		}
		return kindCmd, payload, nil
	case ipc.MagicState:
		rest := make([]byte, ipc.StateHeaderSize-4)
		if _, err := io.ReadFull(r, rest); err != nil {
			return "", nil, err
		}
		sigCount := binary.LittleEndian.Uint16(rest[12:14])  // offset 16 in the full header
		ruleCount := binary.LittleEndian.Uint16(rest[14:16]) // offset 18
		bodyLen := int(sigCount)*ipc.StateSignalItemSize +
			int(ruleCount)*ipc.StateRuleItemSize + ipc.StateSummarySize
		frame := make([]byte, ipc.StateHeaderSize+bodyLen)
		copy(frame[0:4], magicBuf[:])
		copy(frame[4:], rest)
		if _, err := io.ReadFull(r, frame[ipc.StateHeaderSize:]); err != nil {
			return "", nil, err
		}
		return kindState, frame, nil
	default:
		return "", nil, fmt.Errorf("unexpected frame magic 0x%08x", magic)
	}
}

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "ipcsmoke: "+format+"\n", args...)
	os.Exit(1)
}
