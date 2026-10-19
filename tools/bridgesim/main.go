// Command bridgesim simulates cmd/ds5bridge output so the shared-memory profile
// can be tested without the game, Proton or a controller:
//
//	go run ./tools/bridgesim -port 9999
//	./bin/ds5d -dry-run -profile assetto-corsa -trace
package main

import (
	"flag"
	"log"
	"math"
	"net"
	"time"

	"github.com/npc1607/dualsense-c/internal/bridge"
	"github.com/npc1607/dualsense-c/internal/signal"
)

func main() {
	var (
		port = flag.Int("port", 9999, "UDP target port on 127.0.0.1")
		hz   = flag.Int("hz", 60, "send rate in Hz")
		rate = flag.Float64("rate", 1, "speed multiplier for the script")
	)
	flag.Parse()

	addr := &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: *port}
	conn, err := net.DialUDP("udp4", nil, addr)
	if err != nil {
		log.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	fields := []struct {
		name string
		id   uint8
	}{
		{"throttle", mustID("throttle")},
		{"brake", mustID("brake")},
		{"speed", mustID("speed")},
		{"gear", mustID("gear")},
		{"slip", mustID("slip")},
		{"abs", mustID("abs")},
		{"tc", mustID("tc")},
		{"custom0", mustID("custom0")},
	}

	log.Printf("sending simulated bridge packets to %s at %d Hz", addr, *hz)
	ticker := time.NewTicker(time.Second / time.Duration(*hz))
	defer ticker.Stop()
	start := time.Now()

	for now := range ticker.C {
		t := now.Sub(start).Seconds() * *rate
		vals := script(t)

		out := make([]bridge.Field, 0, len(fields))
		for _, f := range fields {
			out = append(out, bridge.Field{ID: f.id, Value: float32(vals[f.name])})
		}
		pkt, err := bridge.Encode(out)
		if err != nil {
			log.Fatalf("encode: %v", err)
		}
		if _, err := conn.Write(pkt); err != nil {
			log.Fatalf("send: %v", err)
		}
	}
}

// script repeats an 8 second throttle/brake cycle with a slip spike and an ABS
// and TC window, so every rule in the profile gets exercised.
func script(t float64) map[string]float64 {
	const cycle = 8.0
	phase := math.Mod(t, cycle)
	v := map[string]float64{"gear": 4, "speed": 120, "custom0": 6500}

	switch {
	case phase < 2:
		v["throttle"] = phase / 2
		v["speed"] = 120 + 60*phase/2
	case phase < 3.5:
		v["throttle"] = 1
		v["speed"] = 180
		v["tc"] = 0.5
	case phase < 4:
		v["throttle"] = 1 - (phase-3.5)/0.5
		v["brake"] = (phase - 3.5) / 0.5
	case phase < 6:
		v["brake"] = 1
		v["speed"] = 180 - 140*(phase-4)/2
		v["abs"] = math.Min(1, (phase-4)/0.5)
		if phase > 4.5 && phase < 5 {
			v["slip"] = 0.8
		}
	case phase < 7:
		v["brake"] = 1 - (phase-6)/1
		v["speed"] = 40
	}
	return v
}

func mustID(name string) uint8 {
	id, ok := signal.ID(name)
	if !ok {
		log.Fatalf("signal %q missing from the vocabulary", name)
	}
	return id
}
