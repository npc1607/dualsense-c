package main

import (
	"context"
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math"
	"net"
	"net/http"
	"os/exec"
	"runtime"
	"strconv"
	"sync"
	"time"

	"github.com/npc1607/dualsense-c/internal/device"
	"github.com/npc1607/dualsense-c/internal/hid"
	"github.com/npc1607/dualsense-c/internal/input"
	"github.com/npc1607/dualsense-c/internal/report"
)

//go:embed web/*
var webFS embed.FS

// GUIState is the complete controller state snapshot pushed to the web client.
type GUIState struct {
	Connected   bool               `json:"connected"`
	DeviceName  string             `json:"device_name"`
	IsSim       bool               `json:"is_sim"`
	Input       input.State        `json:"input"`
	Output      hid.Frame          `json:"output"`
	L2ModeName  string             `json:"l2_mode_name"`
	R2ModeName  string             `json:"r2_mode_name"`
	Signals     map[string]float32 `json:"signals"`
	TimestampMs int64              `json:"timestamp_ms"`
}

// GUICommand is sent from the browser to control the controller hardware.
type GUICommand struct {
	Type       string     `json:"type"` // "test_rumble" | "test_trigger" | "test_light" | "reset"
	RumbleL    *uint8     `json:"rumble_l,omitempty"`
	RumbleR    *uint8     `json:"rumble_r,omitempty"`
	Lightbar   *[3]uint8  `json:"lightbar,omitempty"`
	MuteLED    *bool      `json:"mute_led,omitempty"`
	PlayerLEDs *uint8     `json:"player_leds,omitempty"`
	L2Effect   *TriggerOp `json:"l2_effect,omitempty"`
	R2Effect   *TriggerOp `json:"r2_effect,omitempty"`
}

type TriggerOp struct {
	Mode   string         `json:"mode"`
	Params map[string]int `json:"params"`
}

type guiServer struct {
	dev        device.Device
	isSim      bool
	devName    string
	mu         sync.Mutex
	curInput   input.State
	curOutput  hid.Frame
	l2ModeName string
	r2ModeName string
	clients    map[chan []byte]bool
	clientsMu  sync.Mutex
}

func cmdGUI(args []string) error {
	fs := flag.NewFlagSet("gui", flag.ExitOnError)
	addr := fs.String("addr", "127.0.0.1:8992", "HTTP server listen address")
	hidraw := fs.String("hidraw", "", "device path (/dev/hidrawN)")
	sim := fs.Bool("sim", false, "force simulation mode without hardware")
	noBrowser := fs.Bool("no-browser", false, "do not open browser automatically")
	_ = fs.Parse(args)

	srv := &guiServer{
		l2ModeName: "off",
		r2ModeName: "off",
		clients:    make(map[chan []byte]bool),
		curOutput:  hid.Idle(),
	}

	if *sim {
		srv.isSim = true
		srv.devName = "Virtual DualSense (Simulation Mode)"
	} else {
		d, err := openDevice(*hidraw)
		if err != nil {
			fmt.Printf("[warn] No physical DualSense found (%v)\n", err)
			fmt.Println("[info] Falling back to 3D simulation mode. Connect via USB to use physical controller.")
			srv.isSim = true
			srv.devName = "DualSense (Simulation Demo)"
		} else {
			srv.dev = d
			srv.devName = d.Info().String()
			defer srv.dev.Close()
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Background worker: poll input and write output
	go srv.hardwareLoop(ctx)
	// Background broadcaster: stream SSE state frames at 60 Hz
	go srv.broadcastLoop(ctx)

	mux := http.NewServeMux()
	mux.HandleFunc("/dualsense.glb", func(w http.ResponseWriter, r *http.Request) {
		data, err := webFS.ReadFile("web/dualsense.glb")
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "model/gltf-binary")
		w.Header().Set("Content-Length", strconv.Itoa(len(data)))
		w.Header().Set("Cache-Control", "public, max-age=86400")
		w.Write(data)
	})
	mux.HandleFunc("/", srv.handleIndex)
	mux.HandleFunc("/events", srv.handleEvents)
	mux.HandleFunc("/api/command", srv.handleCommand)
	mux.HandleFunc("/api/state", srv.handleState)

	listener, err := net.Listen("tcp", *addr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", *addr, err)
	}

	url := fmt.Sprintf("http://%s", listener.Addr().String())
	fmt.Printf("\n[ds5ctl] DualSense 3D Visualizer & Hardware Monitor\n")
	fmt.Printf("   Device:   %s\n", srv.devName)
	fmt.Printf("   Web UI:   %s\n\n", url)
	fmt.Println("Press Ctrl+C to stop.")

	if !*noBrowser {
		go openBrowser(url)
	}

	return http.Serve(listener, mux)
}

func (s *guiServer) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	data, err := webFS.ReadFile("web/index.html")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
	w.Write(data)
}

func (s *guiServer) handleEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	msgCh := make(chan []byte, 16)
	s.clientsMu.Lock()
	s.clients[msgCh] = true
	s.clientsMu.Unlock()

	defer func() {
		s.clientsMu.Lock()
		delete(s.clients, msgCh)
		s.clientsMu.Unlock()
	}()

	notify := r.Context().Done()
	for {
		select {
		case <-notify:
			return
		case msg := <-msgCh:
			fmt.Fprintf(w, "data: %s\n\n", msg)
			flusher.Flush()
		}
	}
}

func (s *guiServer) handleState(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	state := s.buildStateLocked()
	s.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(state)
}

func (s *guiServer) handleCommand(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	var cmd GUICommand
	if err := json.NewDecoder(r.Body).Decode(&cmd); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	switch cmd.Type {
	case "test_rumble":
		if cmd.RumbleL != nil {
			s.curOutput.RumbleL = *cmd.RumbleL
		}
		if cmd.RumbleR != nil {
			s.curOutput.RumbleR = *cmd.RumbleR
		}
	case "test_light":
		if cmd.Lightbar != nil {
			s.curOutput.Lightbar = *cmd.Lightbar
		}
		if cmd.MuteLED != nil {
			s.curOutput.MuteLED = *cmd.MuteLED
		}
		if cmd.PlayerLEDs != nil {
			s.curOutput.PlayerLEDs = *cmd.PlayerLEDs
		}
	case "test_trigger":
		if cmd.L2Effect != nil {
			trig, name := buildTriggerFromOp(cmd.L2Effect)
			s.curOutput.L2 = trig
			s.l2ModeName = name
		}
		if cmd.R2Effect != nil {
			trig, name := buildTriggerFromOp(cmd.R2Effect)
			s.curOutput.R2 = trig
			s.r2ModeName = name
		}
	case "reset":
		s.curOutput = hid.Idle()
		s.l2ModeName = "off"
		s.r2ModeName = "off"
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"ok"}`))
}

func buildTriggerFromOp(op *TriggerOp) (hid.Trigger, string) {
	if op == nil || op.Mode == "off" || op.Mode == "" {
		return hid.Trigger{}, "off"
	}
	mode, ok := hid.ParseMode(op.Mode)
	if !ok {
		return hid.Trigger{}, "off"
	}
	var t hid.Trigger
	t.Mode = mode
	for k, v := range op.Params {
		if idx, found := hid.ParamIndex(mode, k); found {
			t.P[idx] = uint8(v)
		}
	}
	return t, op.Mode
}

func (s *guiServer) buildStateLocked() GUIState {
	return GUIState{
		Connected:   !s.isSim && s.dev != nil,
		DeviceName:  s.devName,
		IsSim:       s.isSim,
		Input:       s.curInput,
		Output:      s.curOutput,
		L2ModeName:  s.l2ModeName,
		R2ModeName:  s.r2ModeName,
		Signals:     s.curInput.Signals(),
		TimestampMs: time.Now().UnixMilli(),
	}
}

func (s *guiServer) broadcastLoop(ctx context.Context) {
	ticker := time.NewTicker(time.Second / 60) // 60 Hz update rate
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.mu.Lock()
			state := s.buildStateLocked()
			s.mu.Unlock()

			payload, err := json.Marshal(state)
			if err != nil {
				continue
			}

			s.clientsMu.Lock()
			for ch := range s.clients {
				select {
				case ch <- payload:
				default:
				}
			}
			s.clientsMu.Unlock()
		}
	}
}

func (s *guiServer) hardwareLoop(ctx context.Context) {
	readTicker := time.NewTicker(time.Second / 125) // 125 Hz
	defer readTicker.Stop()

	inBuf := make([]byte, 128)
	simT := 0.0

	for {
		select {
		case <-ctx.Done():
			if s.dev != nil {
				_ = s.dev.Write(report.Build(s.dev.Info(), hid.Idle()))
			}
			return
		case <-readTicker.C:
			if s.isSim {
				// Generate smooth simulated waveforms
				simT += 0.008
				s.mu.Lock()
				s.curInput.LX = float32(math.Sin(simT*1.5)) * 0.7
				s.curInput.LY = float32(math.Cos(simT*1.2)) * 0.7
				s.curInput.L2 = float32((math.Sin(simT*2.0) + 1.0) / 2.0)
				s.curInput.R2 = float32((math.Cos(simT*2.5) + 1.0) / 2.0)
				s.curInput.GyroX = float32(math.Sin(simT*2.0)) * 0.25
				s.curInput.GyroY = float32(math.Cos(simT*1.5)) * 0.35
				s.curInput.GyroZ = float32(math.Sin(simT*1.0)) * 0.2
				s.curInput.Battery = 0.85
				s.curInput.Charging = true
				s.curInput.TouchActive = (int(simT*2)%2 == 0)
				if s.curInput.TouchActive {
					s.curInput.TouchX = float32((math.Sin(simT*3.0) + 1.0) / 2.0)
					s.curInput.TouchY = float32((math.Cos(simT*3.0) + 1.0) / 2.0)
				}
				s.mu.Unlock()
				continue
			}

			// Real hardware path
			if s.dev == nil {
				continue
			}

			n, err := s.dev.Read(inBuf)
			if err == nil && n > 0 {
				if st, ok := input.Parse(inBuf[:n]); ok {
					s.mu.Lock()
					s.curInput = st
					s.mu.Unlock()
				}
			}

			// Write output frame
			s.mu.Lock()
			frame := s.curOutput
			s.mu.Unlock()
			reportBuf := report.Build(s.dev.Info(), frame)
			if err := s.dev.Write(reportBuf); err != nil {
				log.Printf("gui: write error: %v", err)
			}
		}
	}
}

func openBrowser(url string) {
	time.Sleep(150 * time.Millisecond)
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("cmd.exe", "/c", "start", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	_ = cmd.Start()
}
