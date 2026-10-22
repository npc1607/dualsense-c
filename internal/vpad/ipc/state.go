package ipc

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"

	"github.com/npc1607/dualsense-c/internal/rules"
	"github.com/npc1607/dualsense-c/internal/signal"
)

// MagicState identifies a full telemetry state frame ("STT1").
const MagicState uint32 = 0x31545453

// Layout sizing constants for STATE1 wire protocol.
const (
	StateHeaderSize     = 20
	StateSignalItemSize = 11
	StateRuleItemSize   = 13
	StateSummarySize    = 40
)

// StateHeader is the 20-byte preamble of a STATE1 wire frame.
type StateHeader struct {
	Magic       uint32
	Version     uint16
	Flags       uint16
	Seq         uint32
	TickMS      uint32
	SignalCount uint16
	RuleCount   uint16
}

// StateSummary captures output state, hardware status, and health metrics.
type StateSummary struct {
	L2Mode       uint8
	L2Params     [4]uint8
	R2Mode       uint8
	R2Params     [4]uint8
	RumbleLeft   uint8
	RumbleRight  uint8
	LightbarR    uint8
	LightbarG    uint8
	LightbarB    uint8
	PlayerLEDs   uint8
	MuteLED      uint8
	HapticLeft   uint8
	HapticRight  uint8
	RateHz       uint16
	TickJitterUS uint32
	HIDErrors    uint32
	Subscribers  uint16
	AimMismatch  uint8
	Reserved     [8]uint8
}

// EncodeStateFrame serializes signals, rules, and summary into a STATE1 binary frame.
func EncodeStateFrame(hdr StateHeader, sigs []signal.SignalTelemetry, rls []rules.RuleTelemetry, summary StateSummary) []byte {
	sigCount := len(sigs)
	ruleCount := len(rls)

	totalSize := StateHeaderSize + sigCount*StateSignalItemSize + ruleCount*StateRuleItemSize + StateSummarySize
	buf := make([]byte, totalSize)

	// Header
	binary.LittleEndian.PutUint32(buf[0:4], MagicState)
	binary.LittleEndian.PutUint16(buf[4:6], 1) // version
	binary.LittleEndian.PutUint16(buf[6:8], hdr.Flags)
	binary.LittleEndian.PutUint32(buf[8:12], hdr.Seq)
	binary.LittleEndian.PutUint32(buf[12:16], hdr.TickMS)
	binary.LittleEndian.PutUint16(buf[16:18], uint16(sigCount))
	binary.LittleEndian.PutUint16(buf[18:20], uint16(ruleCount))

	offset := StateHeaderSize

	// Signals
	for _, s := range sigs {
		buf[offset] = s.ID
		binary.LittleEndian.PutUint32(buf[offset+1:offset+5], math.Float32bits(float32(s.Shaped)))
		binary.LittleEndian.PutUint32(buf[offset+5:offset+9], math.Float32bits(float32(s.Raw)))
		binary.LittleEndian.PutUint16(buf[offset+9:offset+11], s.AgeMS)
		offset += StateSignalItemSize
	}

	// Rules
	for _, r := range rls {
		binary.LittleEndian.PutUint16(buf[offset:offset+2], r.RuleID)
		buf[offset+2] = r.State
		binary.LittleEndian.PutUint16(buf[offset+3:offset+5], uint16(r.Priority))
		binary.LittleEndian.PutUint32(buf[offset+5:offset+9], r.LastFireMS)
		binary.LittleEndian.PutUint32(buf[offset+9:offset+13], r.UntilMS)
		offset += StateRuleItemSize
	}

	// Summary
	buf[offset] = summary.L2Mode
	copy(buf[offset+1:offset+5], summary.L2Params[:])
	buf[offset+5] = summary.R2Mode
	copy(buf[offset+6:offset+10], summary.R2Params[:])
	buf[offset+10] = summary.RumbleLeft
	buf[offset+11] = summary.RumbleRight
	buf[offset+12] = summary.LightbarR
	buf[offset+13] = summary.LightbarG
	buf[offset+14] = summary.LightbarB
	buf[offset+15] = summary.PlayerLEDs
	buf[offset+16] = summary.MuteLED
	buf[offset+17] = summary.HapticLeft
	buf[offset+18] = summary.HapticRight
	binary.LittleEndian.PutUint16(buf[offset+19:offset+21], summary.RateHz)
	binary.LittleEndian.PutUint32(buf[offset+21:offset+25], summary.TickJitterUS)
	binary.LittleEndian.PutUint32(buf[offset+25:offset+29], summary.HIDErrors)
	binary.LittleEndian.PutUint16(buf[offset+29:offset+31], summary.Subscribers)
	buf[offset+31] = summary.AimMismatch
	copy(buf[offset+32:offset+40], summary.Reserved[:])

	return buf
}

// DecodeStateFrame parses a STATE1 binary frame buffer.
func DecodeStateFrame(b []byte) (StateHeader, []signal.SignalTelemetry, []rules.RuleTelemetry, StateSummary, error) {
	if len(b) < StateHeaderSize+StateSummarySize {
		return StateHeader{}, nil, nil, StateSummary{}, errors.New("vpad/ipc: state frame too short")
	}

	magic := binary.LittleEndian.Uint32(b[0:4])
	if magic != MagicState {
		return StateHeader{}, nil, nil, StateSummary{}, fmt.Errorf("vpad/ipc: invalid state frame magic 0x%08x", magic)
	}

	ver := binary.LittleEndian.Uint16(b[4:6])
	if ver != 1 {
		return StateHeader{}, nil, nil, StateSummary{}, fmt.Errorf("vpad/ipc: unsupported state frame version %d", ver)
	}

	hdr := StateHeader{
		Magic:       magic,
		Version:     ver,
		Flags:       binary.LittleEndian.Uint16(b[6:8]),
		Seq:         binary.LittleEndian.Uint32(b[8:12]),
		TickMS:      binary.LittleEndian.Uint32(b[12:16]),
		SignalCount: binary.LittleEndian.Uint16(b[16:18]),
		RuleCount:   binary.LittleEndian.Uint16(b[18:20]),
	}

	expectedSize := StateHeaderSize + int(hdr.SignalCount)*StateSignalItemSize + int(hdr.RuleCount)*StateRuleItemSize + StateSummarySize
	if len(b) < expectedSize {
		return hdr, nil, nil, StateSummary{}, fmt.Errorf("vpad/ipc: buffer size %d less than expected %d", len(b), expectedSize)
	}

	offset := StateHeaderSize
	sigs := make([]signal.SignalTelemetry, hdr.SignalCount)
	for i := 0; i < int(hdr.SignalCount); i++ {
		id := b[offset]
		shaped := math.Float32frombits(binary.LittleEndian.Uint32(b[offset+1 : offset+5]))
		raw := math.Float32frombits(binary.LittleEndian.Uint32(b[offset+5 : offset+9]))
		ageMS := binary.LittleEndian.Uint16(b[offset+9 : offset+11])

		sigs[i] = signal.SignalTelemetry{
			ID:     id,
			Name:   signal.Name(id),
			Raw:    float64(raw),
			Shaped: float64(shaped),
			AgeMS:  ageMS,
			Fresh:  ageMS != 0xFFFF,
		}
		offset += StateSignalItemSize
	}

	rls := make([]rules.RuleTelemetry, hdr.RuleCount)
	for i := 0; i < int(hdr.RuleCount); i++ {
		ruleID := binary.LittleEndian.Uint16(b[offset : offset+2])
		state := b[offset+2]
		prio := int16(binary.LittleEndian.Uint16(b[offset+3 : offset+5]))
		lastFire := binary.LittleEndian.Uint32(b[offset+5 : offset+9])
		until := binary.LittleEndian.Uint32(b[offset+9 : offset+13])

		rls[i] = rules.RuleTelemetry{
			RuleID:     ruleID,
			State:      state,
			Priority:   prio,
			LastFireMS: lastFire,
			UntilMS:    until,
		}
		offset += StateRuleItemSize
	}

	var summary StateSummary
	summary.L2Mode = b[offset]
	copy(summary.L2Params[:], b[offset+1:offset+5])
	summary.R2Mode = b[offset+5]
	copy(summary.R2Params[:], b[offset+6:offset+10])
	summary.RumbleLeft = b[offset+10]
	summary.RumbleRight = b[offset+11]
	summary.LightbarR = b[offset+12]
	summary.LightbarG = b[offset+13]
	summary.LightbarB = b[offset+14]
	summary.PlayerLEDs = b[offset+15]
	summary.MuteLED = b[offset+16]
	summary.HapticLeft = b[offset+17]
	summary.HapticRight = b[offset+18]
	summary.RateHz = binary.LittleEndian.Uint16(b[offset+19 : offset+21])
	summary.TickJitterUS = binary.LittleEndian.Uint32(b[offset+21 : offset+25])
	summary.HIDErrors = binary.LittleEndian.Uint32(b[offset+25 : offset+29])
	summary.Subscribers = binary.LittleEndian.Uint16(b[offset+29 : offset+31])
	summary.AimMismatch = b[offset+31]
	copy(summary.Reserved[:], b[offset+32:offset+40])

	return hdr, sigs, rls, summary, nil
}

// ReadStateFrame reads a complete STATE1 frame from an io.Reader stream.
func ReadStateFrame(r io.Reader) (StateHeader, []signal.SignalTelemetry, []rules.RuleTelemetry, StateSummary, error) {
	var hdrBuf [StateHeaderSize]byte
	if _, err := io.ReadFull(r, hdrBuf[:]); err != nil {
		return StateHeader{}, nil, nil, StateSummary{}, err
	}
	if binary.LittleEndian.Uint32(hdrBuf[0:4]) != MagicState {
		return StateHeader{}, nil, nil, StateSummary{}, errors.New("vpad/ipc: invalid state frame magic")
	}

	sigCount := binary.LittleEndian.Uint16(hdrBuf[16:18])
	ruleCount := binary.LittleEndian.Uint16(hdrBuf[18:20])

	bodyLen := int(sigCount)*StateSignalItemSize + int(ruleCount)*StateRuleItemSize + StateSummarySize
	body := make([]byte, StateHeaderSize+bodyLen)
	copy(body[0:StateHeaderSize], hdrBuf[:])

	if _, err := io.ReadFull(r, body[StateHeaderSize:]); err != nil {
		return StateHeader{}, nil, nil, StateSummary{}, err
	}

	return DecodeStateFrame(body)
}
