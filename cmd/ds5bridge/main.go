//go:build windows

// Command ds5bridge runs inside the Proton/Wine prefix of a Windows racing
// game, reads the game's shared-memory telemetry page and forwards it to the
// native ds5d daemon over UDP.
//
// Why this has to run inside the prefix: `acpmf_physics` is a Windows named
// section created by the game process. A native Linux process cannot see it,
// but any process in the same Wine prefix can. Everything else (rules, HID) is
// native, so this helper stays deliberately dumb: read offsets, send a packet.
//
// Build (pure Go, no mingw):
//
//	GOOS=windows GOARCH=amd64 go build -o bin/ds5bridge.exe ./cmd/ds5bridge
//
// Run inside the prefix (Linux side):
//
//	./scripts/launch-bridge.sh <steam-appid>
package main

import (
	"errors"
	"flag"
	"log"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/npc1607/dualsense-c/internal/bridge"
	"github.com/npc1607/dualsense-c/internal/signal"
)

var version = "dev"

var (
	kernel32            = windows.NewLazySystemDLL("kernel32.dll")
	procOpenFileMapping = kernel32.NewProc("OpenFileMappingW")
)

// openFileMapping opens an existing named section read-only.
func openFileMapping(name string) (windows.Handle, error) {
	n, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return 0, err
	}
	r1, _, e1 := procOpenFileMapping.Call(
		uintptr(windows.FILE_MAP_READ),
		0, // bInheritHandle = FALSE
		uintptr(unsafe.Pointer(n)),
	)
	if r1 == 0 {
		return 0, e1
	}
	return windows.Handle(r1), nil
}

func main() {
	var (
		layoutName = flag.String("layout", "ac", "shared memory layout: ac | acc | acr")
		section    = flag.String("section", "", "override the section name (default from -layout)")
		port       = flag.Int("port", 9999, "UDP target port on 127.0.0.1")
		hz         = flag.Int("hz", 60, "send rate in Hz")
		debug      = flag.Bool("debug", false, "log telemetry values every 2s")
		events     = flag.Float64("events", 0, "log every frame whose |accel_g| exceeds this (0 = off)")
		showVer    = flag.Bool("version", false, "print version and exit")
	)
	flag.Parse()

	if *showVer {
		log.Printf("ds5bridge %s", version)
		return
	}

	layout, ok := bridge.Layouts[*layoutName]
	if !ok {
		log.Fatalf("unknown layout %q (have: ac, acc, acr)", *layoutName)
	}
	if *section != "" {
		layout.Section = *section
	}
	ids, err := resolveIDs(layout)
	if err != nil {
		log.Fatalf("%v", err)
	}

	// Raw Winsock: Go's net package issues WSAIoctl(SIO_UDP_CONNRESET), which
	// Wine does not implement (winapi error 10045).
	var wsa windows.WSAData
	if err := windows.WSAStartup(0x0202, &wsa); err != nil {
		log.Fatalf("WSAStartup: %v", err)
	}
	defer windows.WSACleanup()

	sock, err := windows.Socket(windows.AF_INET, windows.SOCK_DGRAM, windows.IPPROTO_UDP)
	if err != nil || sock == windows.InvalidHandle {
		log.Fatalf("socket: %v", err)
	}
	defer windows.Closesocket(sock)

	dst := windows.SockaddrInet4{Port: *port, Addr: [4]byte{127, 0, 0, 1}}
	log.Printf("ds5bridge %s: layout %s, section %q -> 127.0.0.1:%d at %d Hz",
		version, layout.Name, layout.Section, *port, *hz)

	for {
		h, base, err := tryOpen(layout.Section)
		if err != nil {
			log.Printf("%v — retrying in 2s (start the game and enter a session)", err)
			time.Sleep(2 * time.Second)
			continue
		}
		log.Printf("opened %q; streaming", layout.Section)
		stream(sock, dst, base, layout, ids, *hz, *debug, *events)

		windows.UnmapViewOfFile(base)
		windows.CloseHandle(h)
		log.Printf("telemetry lost; re-opening")
		time.Sleep(time.Second)
	}
}

func resolveIDs(l bridge.Layout) (map[string]uint8, error) {
	ids := make(map[string]uint8)
	for _, name := range l.Signals() {
		id, ok := signal.ID(name)
		if !ok {
			return nil, errors.New("layout references unknown signal " + name + " (vocabulary drift?)")
		}
		ids[name] = id
	}
	return ids, nil
}

func tryOpen(section string) (windows.Handle, uintptr, error) {
	// The section may be namespaced (Local\) or not, depending on the title.
	for _, name := range []string{`Local\` + section, section} {
		h, err := openFileMapping(name)
		if err != nil {
			continue
		}
		base, err := windows.MapViewOfFile(h, windows.FILE_MAP_READ, 0, 0, 0)
		if err != nil {
			windows.CloseHandle(h)
			continue
		}
		return h, base, nil
	}
	return 0, 0, errors.New(section + " shared memory not found")
}

func stream(sock windows.Handle, dst windows.SockaddrInet4, base uintptr, layout bridge.Layout, ids map[string]uint8, hz int, debug bool, events float64) {
	interval := time.Second / time.Duration(hz)
	seen := make(map[string]float32, len(ids))
	lastDebug := time.Now()
	lastPacket := int32(-1)
	var staleSince time.Time
	badFrames := 0

	for {
		packetID := readI32(base, 0)
		for _, f := range layout.Fields {
			seen[f.Signal] = readValue(base, f)
		}
		layout.Derive(seen)

		if debug && time.Since(lastDebug) >= 2*time.Second {
			log.Printf("packetId=%d throttle=%.2f brake=%.2f speed=%.1f slip=%.2f abs=%.2f tc=%.2f",
				packetID, seen["throttle"], seen["brake"], seen["speed"],
				seen["slip"], seen["abs"], seen["tc"])
			log.Printf("accelG=%.2f latG=%.2f steer=%.2f surface=%.2f damage=%.3f susp=%.3f",
				seen["accel_g"], seen["lat_g"], seen["steer"],
				seen["surface"], seen["custom1"], seen["custom2"])
			lastDebug = time.Now()
		}

		if !plausible(seen) {
			badFrames++
			if badFrames == 1 {
				log.Printf("layout mismatch? packetId=%d throttle=%.2f brake=%.2f speed=%.1f",
					packetID, seen["throttle"], seen["brake"], seen["speed"])
			}
			time.Sleep(interval)
			continue
		}
		badFrames = 0

		// -events logs only the interesting frames (kerb hits, impacts) at full
		// 60 Hz resolution, which -debug's 2 s sampling would miss entirely.
		if events > 0 && float64(absf(seen["accel_g"])) >= events {
			log.Printf("EVENT accelG=%.2f latG=%.2f speed=%.1f thr=%.2f brk=%.2f surface=%.2f damage=%.4f susp=%.3f",
				seen["accel_g"], seen["lat_g"], seen["speed"], seen["throttle"],
				seen["brake"], seen["surface"], seen["custom1"], seen["custom2"])
		}

		// A live game keeps bumping packetId; if it stops for two seconds the
		// session ended (or restarted) and the mapping must be re-opened.
		if packetID != lastPacket {
			lastPacket = packetID
			staleSince = time.Time{}
		} else if staleSince.IsZero() {
			staleSince = time.Now()
		} else if time.Since(staleSince) > 2*time.Second {
			log.Printf("packetId stuck at %d; assuming the session ended", packetID)
			return
		}

		fields := make([]bridge.Field, 0, len(layout.Fields)+len(layout.MaxAbs))
		for _, f := range layout.Fields {
			fields = append(fields, bridge.Field{ID: ids[f.Signal], Value: seen[f.Signal]})
		}
		for _, g := range layout.MaxAbs {
			fields = append(fields, bridge.Field{ID: ids[g.Target], Value: seen[g.Target]})
		}
		payload, err := bridge.Encode(fields)
		if err != nil {
			log.Printf("encode: %v", err)
			return
		}
		if err := windows.Sendto(sock, payload, 0, &dst); err != nil {
			log.Printf("sendto: %v", err)
			return
		}
		time.Sleep(interval)
	}
}

func readValue(base uintptr, f bridge.FieldSpec) float32 {
	var best float32
	for i := 0; i < f.Stride(); i++ {
		var v float32
		if f.Kind == bridge.KindI32 {
			v = float32(readI32(base, f.Off+i*4))
		} else {
			v = readF32(base, f.Off+i*4)
		}
		if i == 0 || absf(v) > absf(best) {
			best = v
		}
	}
	return best * float32(f.Factor())
}

func absf(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}

func readF32(base uintptr, off int) float32 {
	return *(*float32)(unsafe.Pointer(base + uintptr(off)))
}

func readI32(base uintptr, off int) int32 {
	return *(*int32)(unsafe.Pointer(base + uintptr(off)))
}

// plausible rejects garbage so a layout mismatch does not drive the triggers.
func plausible(v map[string]float32) bool {
	inRange := func(name string, lo, hi float32) bool {
		x, ok := v[name]
		return !ok || (x >= lo && x <= hi && x == x) // x == x rejects NaN
	}
	return inRange("throttle", 0, 1.5) &&
		inRange("brake", 0, 1.5) &&
		inRange("speed", 0, 500) &&
		inRange("abs", 0, 1) &&
		inRange("tc", 0, 1)
}
