// Package ipc provides a high-performance, ultra-low latency event-driven
// communication protocol between ds5vpad (provider) and ds5d (consumer).
//
// Protocol Features:
//   - Zero polling jitter: publishes force-feedback rumble events immediately
//     (< 50 microseconds) upon kernel/driver notification, without 60Hz ticker lag.
//   - Zero idle overhead: when no consumer is connected (e.g. games with native
//     telemetry like RBR, Forza, etc.), the provider performs 0 network I/O
//     and outputs 0 error noise.
//   - TCP_NODELAY enabled for instantaneous loopback dispatch.
//   - Connection-oriented with automatic reconnect and multi-subscriber support.
//   - Dedicated per-subscriber worker queues with non-blocking drops to eliminate
//     head-of-line blocking on the 125Hz input thread.
package ipc

import (
	"cmp"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"net"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/npc1607/dualsense-c/internal/signal"
	"github.com/npc1607/dualsense-c/internal/vpad"
)

// Magic identifies an FFB protocol frame ("FFB1").
const Magic uint32 = 0x31424646

// MagicSig identifies a telemetry signal frame ("SIG1").
const MagicSig uint32 = 0x31474953

// MagicSpeech identifies a companion speech broadcast frame ("SPK1").
const MagicSpeech uint32 = 0x31505343

// DefaultSignalEndpoint is the standard loopback IPC address for telemetry broadcasting.
const DefaultSignalEndpoint = "127.0.0.1:9997"

// SigHeaderSize is the 12-byte header for SIG1 frames.
const SigHeaderSize = 12

// SigItemSize is the 5-byte payload item (1-byte ID + 4-byte Float32).
const SigItemSize = 5

// SpeechHeaderSize is the 8-byte header for SPK1 frames (4-byte magic + 4-byte payload size).
const SpeechHeaderSize = 8

// FrameSize is the 12-byte wire format for FFB1:
//
//	0..3:  Magic ("FFB1", little endian uint32 0x31424646)
//	4:     Left motor magnitude (0..255)
//	5:     Right motor magnitude (0..255)
//	6..7:  Reserved (0x00)
//	8..11: Monotonic sequence number (little endian uint32)
const FrameSize = 12

// DefaultEndpoint is the standard loopback IPC address.
const DefaultEndpoint = "127.0.0.1:9998"

// ParseEndpoint splits an endpoint string into network and address.
// Examples:
//   - "127.0.0.1:9998"      -> network: "tcp",  address: "127.0.0.1:9998"
//   - ":9998"               -> network: "tcp",  address: ":9998"
//   - "unix:///tmp/pad.sock"-> network: "unix", address: "/tmp/pad.sock"
//   - "/tmp/pad.sock"       -> network: "unix", address: "/tmp/pad.sock"
func ParseEndpoint(ep string) (network, address string) {
	if ep == "" {
		ep = DefaultEndpoint
	}
	if strings.HasPrefix(ep, "unix://") {
		return "unix", strings.TrimPrefix(ep, "unix://")
	}
	if strings.HasPrefix(ep, "/") || strings.HasPrefix(ep, "@") {
		return "unix", ep
	}
	if strings.HasPrefix(ep, "tcp://") {
		return "tcp", strings.TrimPrefix(ep, "tcp://")
	}
	return "tcp", ep
}

// SpeechFrame represents a companion voice or radio event broadcast over IPC.
type SpeechFrame struct {
	Seq      uint32    `json:"seq"`
	Name     string    `json:"name"`
	Target   string    `json:"target"`
	Clip     string    `json:"clip"`
	Priority int       `json:"priority"`
	Text     string    `json:"text"`
	At       time.Time `json:"at"`
}

// Server provides low-latency event-driven force feedback IPC.
type clientHandle struct {
	conn       net.Conn
	ch         chan []byte
	mu         sync.Mutex
	isRPC      atomic.Bool
	subSignals atomic.Bool
	subState   atomic.Bool
}

type Server struct {
	listener net.Listener
	logger   *log.Logger

	mu      sync.RWMutex
	clients map[net.Conn]*clientHandle
	seq     uint32
	closed  atomic.Bool

	expireMu sync.Mutex
	expire   *time.Timer

	cmdMu      sync.RWMutex
	cmdHandler CommandHandler
}

// Listen creates and starts an IPC server on endpoint.
func Listen(endpoint string, logger *log.Logger) (*Server, error) {
	if logger == nil {
		logger = log.Default()
	}
	netw, addr := ParseEndpoint(endpoint)
	l, err := net.Listen(netw, addr)
	if err != nil {
		return nil, fmt.Errorf("vpad/ipc: listen %s %s: %w", netw, addr, err)
	}

	s := &Server{
		listener: l,
		logger:   logger,
		clients:  make(map[net.Conn]*clientHandle),
	}
	go s.acceptLoop()
	return s, nil
}

// Endpoint returns the bound listener address.
func (s *Server) Endpoint() string {
	if s.listener == nil {
		return ""
	}
	return s.listener.Addr().String()
}

func (s *Server) acceptLoop() {
	for !s.closed.Load() {
		conn, err := s.listener.Accept()
		if err != nil {
			if s.closed.Load() {
				return
			}
			var ne net.Error
			if errors.As(err, &ne) && ne.Timeout() {
				continue
			}
			s.logger.Printf("vpad/ipc: accept error: %v", err)
			return
		}

		if tc, ok := conn.(*net.TCPConn); ok {
			_ = tc.SetNoDelay(true)
			_ = tc.SetKeepAlive(true)
		}

		h := &clientHandle{
			conn: conn,
			ch:   make(chan []byte, 64),
		}

		// Backward-compatibility: if the connected peer does not issue any CMD1 commands
		// within 20ms (e.g. legacy ds5radio or rumble listeners), automatically treat it as a signals subscriber.
		time.AfterFunc(20*time.Millisecond, func() {
			if !h.isRPC.Load() && !h.subState.Load() {
				h.subSignals.Store(true)
			}
		})

		s.mu.Lock()
		s.clients[conn] = h
		cnt := len(s.clients)
		s.mu.Unlock()

		s.logger.Printf("vpad/ipc: client connected from %s (%d active subscriber%s)",
			conn.RemoteAddr(), cnt, plural(cnt))

		go s.clientWriter(h)
		go s.clientReader(h)
	}
}

func (s *Server) clientWriter(h *clientHandle) {
	defer func() {
		s.mu.Lock()
		delete(s.clients, h.conn)
		cnt := len(s.clients)
		s.mu.Unlock()
		_ = h.conn.Close()
		s.logger.Printf("vpad/ipc: disconnected subscriber %s (%d active)",
			h.conn.RemoteAddr(), cnt)
	}()

	for frame := range h.ch {
		_ = h.conn.SetWriteDeadline(time.Now().Add(200 * time.Millisecond))
		h.mu.Lock()
		_, err := h.conn.Write(frame)
		h.mu.Unlock()
		if err != nil {
			return
		}
	}
}

func drainChan(ch chan []byte) {
	for {
		select {
		case <-ch:
		default:
			return
		}
	}
}

func (s *Server) clientReader(h *clientHandle) {
	for !s.closed.Load() {
		cmd, err := ReadCommand(h.conn)
		if err != nil {
			return
		}

		h.isRPC.Store(true)
		switch cmd.Method {
		case "state.subscribe":
			h.subState.Store(true)
			h.subSignals.Store(false)
			drainChan(h.ch)
		case "signals.subscribe":
			h.subSignals.Store(true)
			h.subState.Store(false)
			drainChan(h.ch)
		default:
			if !h.subState.Load() && !h.subSignals.Load() {
				drainChan(h.ch)
			}
		}

		s.cmdMu.RLock()
		handler := s.cmdHandler
		s.cmdMu.RUnlock()

		var resp Response
		if handler != nil {
			resp = handler(*cmd)
		} else {
			resp = Response{
				ID:    cmd.ID,
				OK:    false,
				Error: "no command handler registered",
			}
		}

		_ = h.conn.SetWriteDeadline(time.Now().Add(500 * time.Millisecond))
		h.mu.Lock()
		err = WriteResponse(h.conn, resp)
		h.mu.Unlock()
		if err != nil {
			return
		}
	}
}

func (s *Server) broadcastRaw(frame []byte) {
	s.broadcastRawFiltered(frame, func(h *clientHandle) bool {
		return h.subSignals.Load()
	})
}

func (s *Server) broadcastRawFiltered(frame []byte, match func(h *clientHandle) bool) {
	if s.closed.Load() {
		return
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.closed.Load() {
		return
	}
	for _, h := range s.clients {
		if !match(h) {
			continue
		}
		select {
		case h.ch <- frame:
		default: // Drop immediately if subscriber queue is full; never block 125Hz hot path!
		}
	}
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// Subscribers returns the current count of active subscribers.
func (s *Server) Subscribers() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.clients)
}

// Broadcast pushes a force-feedback rumble event immediately to all active subscribers.
// If no subscribers are connected, it returns immediately without any I/O or log noise.
func (s *Server) Broadcast(r vpad.Rumble) {
	if s.Subscribers() == 0 {
		return
	}

	seq := atomic.AddUint32(&s.seq, 1)
	var frame [FrameSize]byte
	binary.LittleEndian.PutUint32(frame[0:], Magic)
	frame[4] = r.Left
	frame[5] = r.Right
	binary.LittleEndian.PutUint32(frame[8:], seq)

	s.broadcastRaw(frame[:])

	// Schedule decay if active effect stops refreshing
	s.expireMu.Lock()
	if s.expire != nil {
		s.expire.Stop()
	}
	if r.Left > 0 || r.Right > 0 {
		s.expire = time.AfterFunc(500*time.Millisecond, func() {
			s.Broadcast(vpad.Rumble{At: time.Now()})
		})
	}
	s.expireMu.Unlock()
}

type sigPair struct {
	id  uint8
	val float32
}

// EncodeSignalFrame encodes active signals into a SIG1 binary wire frame:
//
//	0..3:  Magic ("SIG1", little endian uint32 0x31474953)
//	4..5:  Version (uint16 LE, 1)
//	6..7:  Signal Count N (uint16 LE)
//	8..11: Monotonic sequence number (uint32 LE)
//	12..:  N * 5 bytes: 1-byte Signal ID + 4-byte Float32 (LE)
func EncodeSignalFrame(seq uint32, signals map[string]float32) []byte {
	pairs := make([]sigPair, 0, len(signals))
	for name, val := range signals {
		if id, ok := signal.ID(name); ok {
			pairs = append(pairs, sigPair{id: id, val: val})
		}
	}
	slices.SortFunc(pairs, func(a, b sigPair) int {
		return cmp.Compare(a.id, b.id)
	})

	buf := make([]byte, SigHeaderSize+len(pairs)*SigItemSize)
	binary.LittleEndian.PutUint32(buf[0:4], MagicSig)
	binary.LittleEndian.PutUint16(buf[4:6], 1)
	binary.LittleEndian.PutUint16(buf[6:8], uint16(len(pairs)))
	binary.LittleEndian.PutUint32(buf[8:12], seq)

	offset := SigHeaderSize
	for _, p := range pairs {
		buf[offset] = p.id
		binary.LittleEndian.PutUint32(buf[offset+1:offset+5], math.Float32bits(p.val))
		offset += SigItemSize
	}
	return buf
}

// DecodeSignalFrame decodes an incoming SIG1 wire frame buffer.
func DecodeSignalFrame(b []byte) (seq uint32, signals map[string]float32, ok bool) {
	if len(b) < SigHeaderSize {
		return 0, nil, false
	}
	if binary.LittleEndian.Uint32(b[0:4]) != MagicSig {
		return 0, nil, false
	}
	ver := binary.LittleEndian.Uint16(b[4:6])
	if ver != 1 {
		return 0, nil, false
	}
	count := binary.LittleEndian.Uint16(b[6:8])
	expected := SigHeaderSize + int(count)*SigItemSize
	if len(b) < expected {
		return 0, nil, false
	}
	seq = binary.LittleEndian.Uint32(b[8:12])
	signals = make(map[string]float32, count)
	offset := SigHeaderSize
	for i := 0; i < int(count); i++ {
		id := b[offset]
		bits := binary.LittleEndian.Uint32(b[offset+1 : offset+5])
		val := math.Float32frombits(bits)
		if name := signal.Name(id); name != "" {
			signals[name] = val
		}
		offset += SigItemSize
	}
	return seq, signals, true
}

// ReadSignalFrame reads exactly one SIG1 telemetry frame from r.
func ReadSignalFrame(r io.Reader) (seq uint32, signals map[string]float32, err error) {
	var hdr [SigHeaderSize]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return 0, nil, err
	}
	if binary.LittleEndian.Uint32(hdr[0:4]) != MagicSig {
		return 0, nil, errors.New("vpad/ipc: invalid signal frame magic")
	}
	ver := binary.LittleEndian.Uint16(hdr[4:6])
	if ver != 1 {
		return 0, nil, fmt.Errorf("vpad/ipc: unsupported signal frame version %d", ver)
	}
	count := binary.LittleEndian.Uint16(hdr[6:8])
	if count > 1024 {
		return 0, nil, fmt.Errorf("vpad/ipc: signal frame count too large (%d)", count)
	}
	seq = binary.LittleEndian.Uint32(hdr[8:12])
	signals = make(map[string]float32, count)
	if count > 0 {
		payload := make([]byte, int(count)*SigItemSize)
		if _, err := io.ReadFull(r, payload); err != nil {
			return 0, nil, err
		}
		offset := 0
		for i := 0; i < int(count); i++ {
			id := payload[offset]
			bits := binary.LittleEndian.Uint32(payload[offset+1 : offset+5])
			val := math.Float32frombits(bits)
			if name := signal.Name(id); name != "" {
				signals[name] = val
			}
			offset += SigItemSize
		}
	}
	return seq, signals, nil
}

// BroadcastSignals pushes a telemetry signal frame immediately to all active subscribers.
// If no subscribers are connected, it returns immediately without memory allocation or I/O.
func (s *Server) BroadcastSignals(signals map[string]float32) {
	if s.Subscribers() == 0 {
		return
	}

	seq := atomic.AddUint32(&s.seq, 1)
	frame := EncodeSignalFrame(seq, signals)
	s.broadcastRaw(frame)
}

// EncodeSpeechFrame encodes a SpeechFrame into an SPK1 binary wire frame:
//
//	0..3:  Magic ("SPK1", little endian uint32 0x31505343)
//	4..7:  Payload length N (uint32 LE)
//	8..:   N bytes JSON payload
func EncodeSpeechFrame(seq uint32, frame SpeechFrame) []byte {
	frame.Seq = seq
	if frame.At.IsZero() {
		frame.At = time.Now()
	}
	raw, _ := json.Marshal(frame)
	buf := make([]byte, SpeechHeaderSize+len(raw))
	binary.LittleEndian.PutUint32(buf[0:4], MagicSpeech)
	binary.LittleEndian.PutUint32(buf[4:8], uint32(len(raw)))
	copy(buf[SpeechHeaderSize:], raw)
	return buf
}

// DecodeSpeechFrame decodes an incoming SPK1 speech frame buffer.
func DecodeSpeechFrame(b []byte) (frame SpeechFrame, ok bool) {
	if len(b) < SpeechHeaderSize {
		return SpeechFrame{}, false
	}
	if binary.LittleEndian.Uint32(b[0:4]) != MagicSpeech {
		return SpeechFrame{}, false
	}
	payloadLen := binary.LittleEndian.Uint32(b[4:8])
	if uint32(len(b)-SpeechHeaderSize) < payloadLen {
		return SpeechFrame{}, false
	}
	if err := json.Unmarshal(b[SpeechHeaderSize:SpeechHeaderSize+payloadLen], &frame); err != nil {
		return SpeechFrame{}, false
	}
	return frame, true
}

// ReadSpeechFrame reads exactly one SPK1 speech event frame from r.
func ReadSpeechFrame(r io.Reader) (SpeechFrame, error) {
	var hdr [SpeechHeaderSize]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return SpeechFrame{}, err
	}
	if binary.LittleEndian.Uint32(hdr[0:4]) != MagicSpeech {
		return SpeechFrame{}, errors.New("vpad/ipc: invalid speech frame magic")
	}
	length := binary.LittleEndian.Uint32(hdr[4:8])
	if length > 65536 {
		return SpeechFrame{}, fmt.Errorf("vpad/ipc: speech frame too large (%d)", length)
	}
	payload := make([]byte, length)
	if _, err := io.ReadFull(r, payload); err != nil {
		return SpeechFrame{}, err
	}
	var frame SpeechFrame
	if err := json.Unmarshal(payload, &frame); err != nil {
		return SpeechFrame{}, fmt.Errorf("vpad/ipc: unmarshal speech frame: %w", err)
	}
	return frame, nil
}

// BroadcastSpeech pushes a companion speech event frame to all active subscribers.
func (s *Server) BroadcastSpeech(frame SpeechFrame) {
	if s.Subscribers() == 0 {
		return
	}

	seq := atomic.AddUint32(&s.seq, 1)
	raw := EncodeSpeechFrame(seq, frame)
	s.broadcastRaw(raw)
}

// Close destroys the listener and closes all connected clients.
func (s *Server) Close() error {
	if s.closed.Swap(true) {
		return nil
	}
	s.expireMu.Lock()
	if s.expire != nil {
		s.expire.Stop()
	}
	s.expireMu.Unlock()

	s.mu.Lock()
	for conn, h := range s.clients {
		_ = conn.Close()
		close(h.ch)
	}
	s.clients = make(map[net.Conn]*clientHandle)
	s.mu.Unlock()

	if s.listener != nil {
		return s.listener.Close()
	}
	return nil
}

// Dial connects to a vpad/ipc Server with low latency and TCP_NODELAY enabled.
func Dial(ctx context.Context, endpoint string) (net.Conn, error) {
	netw, addr := ParseEndpoint(endpoint)
	var d net.Dialer
	conn, err := d.DialContext(ctx, netw, addr)
	if err != nil {
		return nil, err
	}
	if tc, ok := conn.(*net.TCPConn); ok {
		_ = tc.SetNoDelay(true)
	}
	return conn, nil
}

// DecodeFrame decodes an incoming 12-byte FFB wire frame.
func DecodeFrame(b []byte) (left, right float32, seq uint32, ok bool) {
	if len(b) < FrameSize || binary.LittleEndian.Uint32(b[0:]) != Magic {
		return 0, 0, 0, false
	}
	left = float32(b[4]) / 255.0
	right = float32(b[5]) / 255.0
	seq = binary.LittleEndian.Uint32(b[8:])
	return left, right, seq, true
}

// ReadFrame reads exactly one 12-byte FFB protocol frame from r.
func ReadFrame(r io.Reader, buf []byte) (left, right float32, seq uint32, err error) {
	if len(buf) < FrameSize {
		buf = make([]byte, FrameSize)
	}
	if _, err := io.ReadFull(r, buf[:FrameSize]); err != nil {
		return 0, 0, 0, err
	}
	l, rgt, sq, ok := DecodeFrame(buf[:FrameSize])
	if !ok {
		return 0, 0, 0, errors.New("vpad/ipc: invalid frame magic")
	}
	return l, rgt, sq, nil
}

// SetCommandHandler registers a handler for CMD1 JSON-RPC requests.
func (s *Server) SetCommandHandler(h CommandHandler) {
	s.cmdMu.Lock()
	defer s.cmdMu.Unlock()
	s.cmdHandler = h
}

// BroadcastState pushes a STATE1 telemetry state frame immediately to all active state subscribers.
func (s *Server) BroadcastState(frame []byte) {
	if s.Subscribers() == 0 {
		return
	}
	s.broadcastRawFiltered(frame, func(h *clientHandle) bool {
		return h.subState.Load()
	})
}
