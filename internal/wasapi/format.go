// Package wasapi provides pure-Go Windows Core Audio (WASAPI) loopback capture
// and multi-channel PCM rendering without requiring CGO.
package wasapi

import "encoding/binary"

// WAVEFORMATEX format tags and speaker channel masks.
const (
	WaveFormatPCM        = 0x0001
	WaveFormatIEEEFloat  = 0x0003
	WaveFormatExtensible = 0xFFFE

	// SpeakerQuad is SPEAKER_FRONT_LEFT (0x1) | SPEAKER_FRONT_RIGHT (0x2) |
	// SPEAKER_BACK_LEFT (0x10) | SPEAKER_BACK_RIGHT (0x20).
	// On DualSense 4-channel USB audio, channels 1/2 are front (speaker/headset)
	// and channels 3/4 are back/rear (left/right voice coils).
	SpeakerQuad = 0x00000033
)

// KSDATAFORMAT_SUBTYPE_PCM = {00000001-0000-0010-8000-00AA00389B71}.
var subtypePCM = [16]byte{
	0x01, 0x00, 0x00, 0x00,
	0x00, 0x00,
	0x10, 0x00,
	0x80, 0x00, 0x00, 0xAA, 0x00, 0x38, 0x9B, 0x71,
}

// BuildFloat32Format encodes an 18-byte WAVEFORMATEX structure for interleaved
// 32-bit IEEE float PCM at the given sample rate and channel count.
func BuildFloat32Format(rate, channels int) [18]byte {
	var wf [18]byte
	blockAlign := channels * 4
	avgBytes := rate * blockAlign
	binary.LittleEndian.PutUint16(wf[0:], WaveFormatIEEEFloat)
	binary.LittleEndian.PutUint16(wf[2:], uint16(channels))
	binary.LittleEndian.PutUint32(wf[4:], uint32(rate))
	binary.LittleEndian.PutUint32(wf[8:], uint32(avgBytes))
	binary.LittleEndian.PutUint16(wf[12:], uint16(blockAlign))
	binary.LittleEndian.PutUint16(wf[14:], 32)
	binary.LittleEndian.PutUint16(wf[16:], 0)
	return wf
}

// BuildQuadS16Format encodes a 40-byte WAVEFORMATEXTENSIBLE structure for
// 4-channel 16-bit signed integer PCM with quadraphonic channel mapping
// (front-left, front-right, back-left, back-right) so Windows does not remix
// channels 3/4 away from the DualSense voice coils.
func BuildQuadS16Format(rate int) [40]byte {
	var wf [40]byte
	const (
		channels   = 4
		bits       = 16
		blockAlign = channels * (bits / 8) // 8 bytes per frame
	)
	avgBytes := rate * blockAlign
	binary.LittleEndian.PutUint16(wf[0:], WaveFormatExtensible)
	binary.LittleEndian.PutUint16(wf[2:], channels)
	binary.LittleEndian.PutUint32(wf[4:], uint32(rate))
	binary.LittleEndian.PutUint32(wf[8:], uint32(avgBytes))
	binary.LittleEndian.PutUint16(wf[12:], blockAlign)
	binary.LittleEndian.PutUint16(wf[14:], bits)
	binary.LittleEndian.PutUint16(wf[16:], 22) // sizeof(WAVEFORMATEXTENSIBLE) - sizeof(WAVEFORMATEX)
	binary.LittleEndian.PutUint16(wf[18:], bits)
	binary.LittleEndian.PutUint32(wf[20:], SpeakerQuad)
	copy(wf[24:], subtypePCM[:])
	return wf
}
