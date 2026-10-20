package wasapi

import (
	"encoding/binary"
	"testing"
)

func TestBuildFloat32Format(t *testing.T) {
	wf := BuildFloat32Format(24000, 2)
	if got := binary.LittleEndian.Uint16(wf[0:]); got != WaveFormatIEEEFloat {
		t.Errorf("wFormatTag = %#x, want %#x", got, WaveFormatIEEEFloat)
	}
	if got := binary.LittleEndian.Uint16(wf[2:]); got != 2 {
		t.Errorf("nChannels = %d, want 2", got)
	}
	if got := binary.LittleEndian.Uint32(wf[4:]); got != 24000 {
		t.Errorf("nSamplesPerSec = %d, want 24000", got)
	}
	if got := binary.LittleEndian.Uint32(wf[8:]); got != 24000*8 {
		t.Errorf("nAvgBytesPerSec = %d, want %d", got, 24000*8)
	}
	if got := binary.LittleEndian.Uint16(wf[12:]); got != 8 {
		t.Errorf("nBlockAlign = %d, want 8", got)
	}
	if got := binary.LittleEndian.Uint16(wf[14:]); got != 32 {
		t.Errorf("wBitsPerSample = %d, want 32", got)
	}
}

func TestBuildQuadS16Format(t *testing.T) {
	wf := BuildQuadS16Format(48000)
	if got := binary.LittleEndian.Uint16(wf[0:]); got != WaveFormatExtensible {
		t.Errorf("wFormatTag = %#x, want %#x", got, WaveFormatExtensible)
	}
	if got := binary.LittleEndian.Uint16(wf[2:]); got != 4 {
		t.Errorf("nChannels = %d, want 4", got)
	}
	if got := binary.LittleEndian.Uint32(wf[4:]); got != 48000 {
		t.Errorf("nSamplesPerSec = %d, want 48000", got)
	}
	if got := binary.LittleEndian.Uint32(wf[8:]); got != 48000*8 {
		t.Errorf("nAvgBytesPerSec = %d, want %d", got, 48000*8)
	}
	if got := binary.LittleEndian.Uint16(wf[12:]); got != 8 {
		t.Errorf("nBlockAlign = %d, want 8", got)
	}
	if got := binary.LittleEndian.Uint16(wf[14:]); got != 16 {
		t.Errorf("wBitsPerSample = %d, want 16", got)
	}
	if got := binary.LittleEndian.Uint16(wf[16:]); got != 22 {
		t.Errorf("cbSize = %d, want 22", got)
	}
	if got := binary.LittleEndian.Uint16(wf[18:]); got != 16 {
		t.Errorf("wValidBitsPerSample = %d, want 16", got)
	}
	if got := binary.LittleEndian.Uint32(wf[20:]); got != SpeakerQuad {
		t.Errorf("dwChannelMask = %#x, want %#x", got, SpeakerQuad)
	}
	if got := binary.LittleEndian.Uint32(wf[24:]); got != WaveFormatPCM {
		t.Errorf("SubFormat.Data1 = %#x, want %#x", got, WaveFormatPCM)
	}
}
