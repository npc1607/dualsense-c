package haptics

import (
	"encoding/binary"
	"math"
	"testing"
)

func newTestWriter() *Writer {
	return &Writer{cfg: Config{CarrierHz: 55, Gain: 0.55}}
}

// The first two channels are the controller's speaker/headset; leaking the
// haptic waveform into them would play a buzzing noise out of the DS5 speaker.
func TestSpeakerChannelsStaySilent(t *testing.T) {
	w := newTestWriter()
	w.Set(255, 255)
	buf := make([]byte, 240*8)
	for i := 0; i < 10; i++ {
		w.fill(buf)
	}
	for i := 0; i < 240; i++ {
		ch1 := int16(binary.LittleEndian.Uint16(buf[i*8:]))
		ch2 := int16(binary.LittleEndian.Uint16(buf[i*8+2:]))
		if ch1 != 0 || ch2 != 0 {
			t.Fatalf("frame %d: ch1/ch2 = %d/%d, want 0/0", i, ch1, ch2)
		}
	}
}

func TestCoilChannelsFollowAmplitude(t *testing.T) {
	peak := func(left, right uint8) (int16, int16) {
		w := newTestWriter()
		w.Set(left, right)
		buf := make([]byte, 240*8)
		var pL, pR int16
		for i := 0; i < 10; i++ { // let the smoothing settle
			w.fill(buf)
			for j := 0; j < 240; j++ {
				l := int16(binary.LittleEndian.Uint16(buf[j*8+4:]))
				r := int16(binary.LittleEndian.Uint16(buf[j*8+6:]))
				if l > pL {
					pL = l
				}
				if r > pR {
					pR = r
				}
			}
		}
		return pL, pR
	}

	if l, r := peak(0, 0); l != 0 || r != 0 {
		t.Errorf("silent input produced %d/%d, want 0/0", l, r)
	}

	fullL, fullR := peak(255, 255)
	if fullL <= 0 || fullR <= 0 {
		t.Fatalf("full amplitude produced %d/%d, want both positive", fullL, fullR)
	}

	halfL, _ := peak(128, 0)
	if halfL >= fullL {
		t.Errorf("half amplitude peak %d should be below full %d", halfL, fullL)
	}
	if halfL == 0 {
		t.Errorf("half amplitude produced silence")
	}
}

// Left and right coils must be independent so kerbs can hit one side.
func TestChannelsAreIndependent(t *testing.T) {
	w := newTestWriter()
	w.Set(255, 0)
	buf := make([]byte, 240*8)
	var left, right int16
	for i := 0; i < 10; i++ {
		w.fill(buf)
		for j := 0; j < 240; j++ {
			if l := int16(binary.LittleEndian.Uint16(buf[j*8+4:])); l > left {
				left = l
			}
			if r := int16(binary.LittleEndian.Uint16(buf[j*8+6:])); r > right {
				right = r
			}
		}
	}
	if left == 0 {
		t.Error("left coil silent")
	}
	if right > 0 {
		t.Errorf("right coil = %d while amplitude 0", right)
	}
}

// Passthrough mode must (a) band-limit the game audio and (b) scale it by the
// rule gain, so the coils reproduce the real spectrum instead of a carrier.
func TestPassthroughFiltersAndScales(t *testing.T) {
	w := &Writer{cfg: Config{Gain: 1.0, LowHz: 25, HighHz: 200}}
	for i := range w.pasL {
		w.pasL[i] = highPass(w.cfg.LowHz, SampleRate)
		w.pasR[i] = highPass(w.cfg.LowHz, SampleRate)
	}
	w.pasL[2] = lowPass(w.cfg.HighHz, SampleRate)
	w.pasR[2] = lowPass(w.cfg.HighHz, SampleRate)

	// 60 Hz (in band) must survive; 5 kHz (out of band) must be attenuated.
	peak := func(hz float64, gain uint8) float64 {
		w.ampL.Store(uint32(gain))
		w.ampR.Store(uint32(gain))
		w.smoothL, w.smoothR = float64(gain)/255, float64(gain)/255
		src := make([]float32, 480*2)
		dst := make([]byte, 480*8)
		var p float64
		for block := 0; block < 40; block++ {
			for i := 0; i < 480; i++ {
				n := float64(block*480 + i)
				v := float32(0.5 * math.Sin(2*math.Pi*hz*n/SampleRate))
				src[i*2], src[i*2+1] = v, v
			}
			w.passBlock(dst, src, 480)
			if block < 10 {
				continue // let the filter settle before measuring
			}
			for i := 0; i < 480; i++ {
				l := math.Abs(float64(int16(binary.LittleEndian.Uint16(dst[i*8+4:]))))
				if l > p {
					p = l
				}
			}
		}
		return p
	}

	inBand := peak(60, 255)
	outBand := peak(5000, 255)
	if inBand < 5000 {
		t.Errorf("60 Hz peak = %.0f, want the in-band signal to pass", inBand)
	}
	if outBand > inBand*0.2 {
		t.Errorf("5 kHz peak = %.0f, want it filtered out (in-band %.0f)", outBand, inBand)
	}

	// Gain must scale the output.
	quiet := peak(60, 40)
	if quiet >= inBand*0.5 {
		t.Errorf("gain 40 peak = %.0f should be well below full-scale %.0f", quiet, inBand)
	}
}
