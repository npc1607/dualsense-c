package radio

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/npc1607/dualsense-c/internal/profile"
	"github.com/npc1607/dualsense-c/internal/signal"
	"github.com/npc1607/dualsense-c/internal/source"
	_ "github.com/npc1607/dualsense-c/internal/source/all"
	"github.com/npc1607/dualsense-c/internal/source/udp"
	"github.com/npc1607/dualsense-c/internal/vpad/ipc"
)

// Trigger defines a condition under which a radio clip is broadcast (legacy compatibility).
type Trigger struct {
	Name     string
	Clip     string
	Cooldown time.Duration
	Hold     time.Duration
	Evaluate func(vals map[string]float32, dt time.Duration, state *TriggerState) bool
}

// TriggerState tracks persistence and timing for evaluation (legacy compatibility).
type TriggerState struct {
	HoldDuration time.Duration
	LastFired    time.Time
}

// Monitor watches live telemetry and broadcasts engineer radio lines.
type Monitor struct {
	player       *Player
	classifier   Classifier
	arbitrator   *Arbitrator
	window       *TelemetryWindow
	logger       *log.Logger
	triggers     []Trigger
	states       map[string]*TriggerState
	mu           sync.Mutex
	notifyCh     chan struct{}
	workerCtx    context.Context
	workerCancel context.CancelFunc
}

// NewMonitor initializes a telemetry radio monitor.
func NewMonitor(player *Player, logger *log.Logger) *Monitor {
	if logger == nil {
		logger = log.Default()
	}
	wCtx, wCancel := context.WithCancel(context.Background())
	m := &Monitor{
		player:       player,
		classifier:   NewRuleClassifier(),
		arbitrator:   NewArbitrator(2 * time.Second),
		window:       NewTelemetryWindow(5 * time.Second),
		logger:       logger,
		states:       make(map[string]*TriggerState),
		triggers:     DefaultTriggers(),
		notifyCh:     make(chan struct{}, 1),
		workerCtx:    wCtx,
		workerCancel: wCancel,
	}
	go m.inferenceWorker(wCtx)
	return m
}

// Close stops the background inference worker.
func (m *Monitor) Close() error {
	m.workerCancel()
	return nil
}

func (m *Monitor) inferenceWorker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-m.notifyCh:
		}

		m.mu.Lock()
		classifier := m.classifier
		arbitrator := m.arbitrator
		player := m.player
		m.mu.Unlock()

		if classifier == nil {
			continue
		}

		intent, err := classifier.Classify(ctx, m.window)
		if err != nil {
			if ctx.Err() == nil && m.logger != nil {
				m.logger.Printf("radio/monitor: classify error: %v", err)
			}
			continue
		}
		if intent == nil || arbitrator == nil || player == nil {
			continue
		}

		now := time.Now()
		if arbitrator.TryPlay(intent, now, 3*time.Second) {
			if m.logger != nil {
				m.logger.Printf("radio/monitor: INTENT [%s (pri=%d)] -> broadcasting %q",
					intent.ID, intent.Priority, intent.ClipName)
			}
			go func(it *RadioIntent) {
				if err := player.PlayWithPriority(ctx, it.ClipName, it.Priority); err != nil {
					if m.logger != nil {
						m.logger.Printf("radio/monitor: play %q (pri %d): %v", it.ClipName, it.Priority, err)
					}
					arbitrator.Rollback(it)
					return
				}
				arbitrator.MarkFinished(it, time.Now())
			}(intent)
		}
	}
}

// SetClassifier configures the inference engine.
func (m *Monitor) SetClassifier(c Classifier) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.classifier = c
}

// SetArbitrator configures the radio arbitrator.
func (m *Monitor) SetArbitrator(a *Arbitrator) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.arbitrator = a
}

// Window returns the internal sliding telemetry window.
func (m *Monitor) Window() *TelemetryWindow {
	return m.window
}

// DefaultTriggers returns the standard motorsport race engineer rules.
func DefaultTriggers() []Trigger {
	return []Trigger{
		{
			Name:     "tire_slip",
			Clip:     "tire_slip",
			Cooldown: 10 * time.Second,
			Evaluate: func(vals map[string]float32, _ time.Duration, _ *TriggerState) bool {
				speed := vals["speed"]
				slip := vals["slip"]
				slipFront := maxf(vals["slip_fl"], vals["slip_fr"])
				return speed > 20 && (slip > 1.2 || slipFront > 1.5)
			},
		},
		{
			Name:     "engine_redline",
			Clip:     "engine_redline",
			Cooldown: 14 * time.Second,
			Evaluate: func(vals map[string]float32, dt time.Duration, s *TriggerState) bool {
				rpm := vals["custom0"]
				if rpm > 7200 {
					s.HoldDuration += dt
				} else {
					s.HoldDuration = 0
				}
				// Must hold above 7200 RPM for 1.2 seconds to confirm overdue shift
				return s.HoldDuration >= 1200*time.Millisecond
			},
		},
		{
			Name:     "heavy_crash",
			Clip:     "heavy_crash",
			Cooldown: 15 * time.Second,
			Evaluate: func(vals map[string]float32, _ time.Duration, _ *TriggerState) bool {
				accelG := absf(vals["accel_g"])
				speed := vals["speed"]
				return speed > 15 && accelG > 0.45 && accelG < 30
			},
		},
		{
			Name:     "fastest_pace",
			Clip:     "fastest_pace",
			Cooldown: 45 * time.Second,
			Evaluate: func(vals map[string]float32, dt time.Duration, s *TriggerState) bool {
				speed := vals["speed"]
				slip := vals["slip"]
				if speed > 115 && slip < 0.25 {
					s.HoldDuration += dt
				} else {
					s.HoldDuration = 0
				}
				// 3.5 seconds of high-speed clean run
				return s.HoldDuration >= 3500*time.Millisecond
			},
		},
	}
}

func (m *Monitor) handleFrame(ctx context.Context, f signal.Frame) {
	m.window.Push(f)

	m.mu.Lock()
	hasClassifier := m.classifier != nil
	m.mu.Unlock()

	if hasClassifier {
		select {
		case m.notifyCh <- struct{}{}:
		default:
		}
		return
	}

	// Fallback to legacy triggers if classifier is nil
	now := f.At
	if now.IsZero() {
		now = time.Now()
	}
	m.processFrame(ctx, f.Values, 16*time.Millisecond, now)
}

// RunIPC subscribes to the ds5d IPC telemetry broadcast socket and processes frames until ctx is cancelled.
func (m *Monitor) RunIPC(ctx context.Context, endpoint string) error {
	m.logger.Printf("radio/monitor: subscribing to ds5d IPC telemetry on %s...", endpoint)

	backoff := 500 * time.Millisecond
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		conn, err := ipc.Dial(ctx, endpoint)
		if err != nil {
			m.logger.Printf("radio/monitor: dial %s: %v (retrying in %v)", endpoint, err, backoff)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
				if backoff < 2*time.Second {
					backoff *= 2
				}
				continue
			}
		}

		backoff = 500 * time.Millisecond
		m.logger.Printf("radio/monitor: connected to ds5d IPC broadcaster (%s)", endpoint)

		done := make(chan struct{})
		var closeOnce sync.Once
		cleanup := func() {
			closeOnce.Do(func() {
				close(done)
				_ = conn.Close()
			})
		}
		go func() {
			select {
			case <-ctx.Done():
				cleanup()
			case <-done:
			}
		}()

		for {
			if ctx.Err() != nil {
				cleanup()
				return ctx.Err()
			}
			_, signals, err := ipc.ReadSignalFrame(conn)
			if err != nil {
				cleanup()
				if ctx.Err() != nil {
					return ctx.Err()
				}
				m.logger.Printf("radio/monitor: IPC connection closed (%v), reconnecting...", err)
				break
			}

			frame := signal.Frame{
				Values: signals,
				At:     time.Now(),
			}
			m.handleFrame(ctx, frame)
		}
		cleanup()

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff):
		}
	}
}

// RunUDP listens on a UDP telemetry address and evaluates frames until ctx is cancelled.
func (m *Monitor) RunUDP(ctx context.Context, listenAddr string, decoderName string) error {
	m.logger.Printf("radio/monitor: listening on %s (decoder: %s)", listenAddr, decoderName)

	src, err := udp.New(profile.Source{
		Type: "udp",
		Params: map[string]any{
			"listen":  listenAddr,
			"decoder": decoderName,
			"rate_hz": 60.0,
		},
	}, source.Deps{Logger: m.logger})
	if err != nil {
		return fmt.Errorf("radio/monitor: create udp source: %w", err)
	}

	frames := make(chan signal.Frame, 64)
	go func() {
		if err := src.Run(ctx, frames); err != nil && ctx.Err() == nil {
			m.logger.Printf("radio/monitor: source stopped: %v", err)
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return nil
		case f, ok := <-frames:
			if !ok {
				return nil
			}
			m.handleFrame(ctx, f)
		}
	}
}

// Run listens on a UDP telemetry address and evaluates triggers until ctx is cancelled.
// Provided for backwards compatibility with earlier versions.
func (m *Monitor) Run(ctx context.Context, listenAddr string, decoderName string) error {
	return m.RunUDP(ctx, listenAddr, decoderName)
}

func (m *Monitor) processFrame(ctx context.Context, vals map[string]float32, dt time.Duration, now time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, trig := range m.triggers {
		st, ok := m.states[trig.Name]
		if !ok {
			st = &TriggerState{}
			m.states[trig.Name] = st
		}

		if now.Sub(st.LastFired) < trig.Cooldown {
			continue
		}

		if trig.Evaluate(vals, dt, st) {
			if trig.Hold > 0 {
				st.HoldDuration += dt
				if st.HoldDuration < trig.Hold {
					continue
				}
			}
			st.LastFired = now
			st.HoldDuration = 0
			if m.logger != nil {
				m.logger.Printf("radio/monitor: EVENT [%s] -> broadcasting clip %q", trig.Name, trig.Clip)
			}
			go func(clip string) {
				if m.player != nil {
					_ = m.player.Play(context.Background(), clip)
				}
			}(trig.Clip)
			break // Only fire one radio message at a time
		} else {
			if trig.Hold > 0 {
				st.HoldDuration = 0
			}
		}
	}
}

func absf(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}

func maxf(a, b float32) float32 {
	if a > b {
		return a
	}
	return b
}
