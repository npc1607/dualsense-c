package radio

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"strings"
	"time"
)

// AcousticStyle defines the DSP acoustic rendering profile applied to companion voice audio.
type AcousticStyle string

const (
	// StyleRadio simulates an F1 / rally comms bandpass-limited radio with analog saturation.
	StyleRadio AcousticStyle = "radio"

	// StyleStudio provides transparent flat PCM with soft-peak limiting and normalization.
	StyleStudio AcousticStyle = "studio"

	// StyleMech renders a robotic tactical synthesized filter with ring modulation and metallic comb delay.
	StyleMech AcousticStyle = "mech"

	// StyleWhisper renders an intimate, breathy, de-resonated voice with airy high shelf.
	StyleWhisper AcousticStyle = "whisper"
)

// ParseStyle parses a string into an AcousticStyle.
func ParseStyle(s string) AcousticStyle {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "radio":
		return StyleRadio
	case "studio":
		return StyleStudio
	case "mech":
		return StyleMech
	case "whisper":
		return StyleWhisper
	default:
		return StyleRadio
	}
}

type biquad struct {
	b0, b1, b2 float64
	a1, a2     float64
	x1, x2     float64
	y1, y2     float64
}

func newLowPass(hz, rate, q float64) biquad {
	if q <= 0 {
		q = 0.7071
	}
	if hz >= rate*0.49 {
		hz = rate * 0.49
	}
	if hz < 10 {
		hz = 10
	}
	w0 := 2.0 * math.Pi * hz / rate
	cosW := math.Cos(w0)
	sinW := math.Sin(w0)
	alpha := sinW / (2.0 * q)

	b0 := (1.0 - cosW) / 2.0
	b1 := 1.0 - cosW
	b2 := (1.0 - cosW) / 2.0
	a0 := 1.0 + alpha
	a1 := -2.0 * cosW
	a2 := 1.0 - alpha

	return biquad{
		b0: b0 / a0,
		b1: b1 / a0,
		b2: b2 / a0,
		a1: a1 / a0,
		a2: a2 / a0,
	}
}

func newHighPass(hz, rate, q float64) biquad {
	if q <= 0 {
		q = 0.7071
	}
	if hz >= rate*0.49 {
		hz = rate * 0.49
	}
	if hz < 10 {
		hz = 10
	}
	w0 := 2.0 * math.Pi * hz / rate
	cosW := math.Cos(w0)
	sinW := math.Sin(w0)
	alpha := sinW / (2.0 * q)

	b0 := (1.0 + cosW) / 2.0
	b1 := -(1.0 + cosW)
	b2 := (1.0 + cosW) / 2.0
	a0 := 1.0 + alpha
	a1 := -2.0 * cosW
	a2 := 1.0 - alpha

	return biquad{
		b0: b0 / a0,
		b1: b1 / a0,
		b2: b2 / a0,
		a1: a1 / a0,
		a2: a2 / a0,
	}
}

func (b *biquad) step(x float64) float64 {
	y := b.b0*x + b.b1*b.x1 + b.b2*b.x2 - b.a1*b.y1 - b.a2*b.y2
	if math.Abs(y) < 1e-15 {
		y = 0
	}
	b.x2 = b.x1
	b.x1 = x
	b.y2 = b.y1
	b.y1 = y
	return y
}

// RenderPCM processes a float32 mono PCM buffer according to the chosen AcousticStyle.
func RenderPCM(samples []float32, sampleRate int, style AcousticStyle) []float32 {
	if len(samples) == 0 {
		return nil
	}
	if sampleRate <= 0 {
		sampleRate = 48000
	}

	rate := float64(sampleRate)
	out := make([]float32, len(samples))

	switch style {
	case StyleRadio:
		hp := newHighPass(350, rate, 0.7071)
		lp1 := newLowPass(3200, rate, 0.7071)
		lp2 := newLowPass(3500, rate, 0.7071)
		for i, s := range samples {
			x := float64(s)
			y := hp.step(x)
			y = lp1.step(y)
			// Non-linear tanh analog radio saturation (+30% gain boost)
			y = math.Tanh(y*2.8) * 0.98
			y = lp2.step(y)
			out[i] = float32(clampSample(y))
		}

	case StyleStudio:
		maxPeak := 0.0
		for _, s := range samples {
			p := math.Abs(float64(s))
			if p > maxPeak {
				maxPeak = p
			}
		}
		gain := 1.0
		if maxPeak > 0.95 {
			gain = 0.95 / maxPeak
		}
		for i, s := range samples {
			y := float64(s) * gain
			out[i] = float32(clampSample(y))
		}

	case StyleMech:
		delayLen := int(0.0055 * rate)
		if delayLen < 1 {
			delayLen = 1
		}
		delayBuf := make([]float64, delayLen)
		delayIdx := 0
		lp := newLowPass(4500, rate, 0.7071)

		for i, s := range samples {
			t := float64(i) / rate
			carrier := 0.65 + 0.35*math.Sin(2.0*math.Pi*105.0*t)
			modX := float64(s) * carrier

			delayed := delayBuf[delayIdx]
			comb := modX + 0.45*delayed
			delayBuf[delayIdx] = comb
			delayIdx = (delayIdx + 1) % delayLen

			y := lp.step(comb)
			y = math.Tanh(y) * 0.95
			out[i] = float32(clampSample(y))
		}

	case StyleWhisper:
		hp1 := newHighPass(250, rate, 0.7071)
		hp2 := newHighPass(250, rate, 0.7071)
		hp3 := newHighPass(250, rate, 0.7071)
		hpAir := newHighPass(4800, rate, 0.7071)

		for i, s := range samples {
			x := float64(s)
			y := hp1.step(x)
			y = hp2.step(y)
			y = hp3.step(y)

			air := hpAir.step(y)
			whisper := y + 0.8*air

			// Upward dynamic expansion
			sign := 1.0
			if whisper < 0 {
				sign = -1.0
			}
			absVal := math.Abs(whisper)
			expanded := sign * math.Pow(absVal, 0.75)
			out[i] = float32(clampSample(math.Tanh(expanded * 0.85)))
		}

	default:
		copy(out, samples)
	}

	return out
}

func clampSample(v float64) float64 {
	if v > 1.0 {
		return 1.0
	}
	if v < -1.0 {
		return -1.0
	}
	return v
}

// ReadWAV reads standard PCM or IEEE float WAV audio data from r and converts to mono float32.
func ReadWAV(r io.Reader) (samples []float32, sampleRate int, err error) {
	var riffHdr [12]byte
	if _, err := io.ReadFull(r, riffHdr[:]); err != nil {
		return nil, 0, err
	}
	if string(riffHdr[0:4]) != "RIFF" || string(riffHdr[8:12]) != "WAVE" {
		return nil, 0, errors.New("invalid WAV file: missing RIFF/WAVE header")
	}

	var audioFormat uint16
	var numChannels uint16
	var bitsPerSample uint16
	var sRate uint32
	var foundData bool

	for {
		var chunkHdr [8]byte
		if _, err := io.ReadFull(r, chunkHdr[:]); err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				break
			}
			return nil, 0, err
		}
		chunkID := string(chunkHdr[0:4])
		chunkSize := binary.LittleEndian.Uint32(chunkHdr[4:8])

		pad := chunkSize % 2

		switch chunkID {
		case "fmt ":
			if chunkSize < 16 {
				return nil, 0, errors.New("invalid fmt chunk size")
			}
			var fmtBuf [16]byte
			if _, err := io.ReadFull(r, fmtBuf[:]); err != nil {
				return nil, 0, err
			}
			audioFormat = binary.LittleEndian.Uint16(fmtBuf[0:2])
			numChannels = binary.LittleEndian.Uint16(fmtBuf[2:4])
			sRate = binary.LittleEndian.Uint32(fmtBuf[4:8])
			bitsPerSample = binary.LittleEndian.Uint16(fmtBuf[14:16])
			sampleRate = int(sRate)

			remaining := int(chunkSize) - 16 + int(pad)
			if remaining > 0 {
				if _, err := io.CopyN(io.Discard, r, int64(remaining)); err != nil {
					return nil, 0, err
				}
			}

		case "data":
			foundData = true
			if numChannels == 0 {
				numChannels = 1
			}
			bytesPerSample := int(bitsPerSample / 8)
			if bytesPerSample <= 0 {
				bytesPerSample = 2
			}
			frameBytes := int(numChannels) * bytesPerSample
			numFrames := int(chunkSize) / frameBytes

			data := make([]byte, chunkSize)
			if _, err := io.ReadFull(r, data); err != nil {
				return nil, 0, err
			}
			if pad > 0 {
				var p [1]byte
				_, _ = io.ReadFull(r, p[:])
			}

			samples = make([]float32, numFrames)
			for i := 0; i < numFrames; i++ {
				var sum float32
				for ch := 0; ch < int(numChannels); ch++ {
					offset := i*frameBytes + ch*bytesPerSample
					var val float32
					if audioFormat == 3 && bitsPerSample == 32 {
						bits := binary.LittleEndian.Uint32(data[offset : offset+4])
						val = math.Float32frombits(bits)
					} else if bitsPerSample == 16 {
						raw := int16(binary.LittleEndian.Uint16(data[offset : offset+2]))
						val = float32(raw) / 32768.0
					} else if bitsPerSample == 8 {
						val = (float32(data[offset]) - 128.0) / 128.0
					}
					sum += val
				}
				samples[i] = sum / float32(numChannels)
			}
			return samples, sampleRate, nil

		default:
			skip := int64(chunkSize) + int64(pad)
			if _, err := io.CopyN(io.Discard, r, skip); err != nil {
				return nil, 0, err
			}
		}
	}

	if !foundData {
		return nil, 0, errors.New("WAV data chunk not found")
	}
	return samples, sampleRate, nil
}

// WriteWAV writes mono 16-bit PCM WAV audio data to w.
func WriteWAV(w io.Writer, samples []float32, sampleRate int) error {
	if sampleRate <= 0 {
		sampleRate = 48000
	}
	numSamples := len(samples)
	dataSize := uint32(numSamples * 2)
	fileSize := 36 + dataSize

	var hdr [44]byte
	copy(hdr[0:4], "RIFF")
	binary.LittleEndian.PutUint32(hdr[4:8], fileSize)
	copy(hdr[8:12], "WAVE")
	copy(hdr[12:16], "fmt ")
	binary.LittleEndian.PutUint32(hdr[16:20], 16) // fmt chunk size
	binary.LittleEndian.PutUint16(hdr[20:22], 1)  // PCM
	binary.LittleEndian.PutUint16(hdr[22:24], 1)  // Mono
	binary.LittleEndian.PutUint32(hdr[24:28], uint32(sampleRate))
	binary.LittleEndian.PutUint32(hdr[28:32], uint32(sampleRate*2)) // byte rate
	binary.LittleEndian.PutUint16(hdr[32:34], 2)                    // block align
	binary.LittleEndian.PutUint16(hdr[34:36], 16)                   // bits per sample
	copy(hdr[36:40], "data")
	binary.LittleEndian.PutUint32(hdr[40:44], dataSize)

	if _, err := w.Write(hdr[:]); err != nil {
		return err
	}

	buf := make([]byte, numSamples*2)
	for i, s := range samples {
		clamped := clampSample(float64(s))
		val := int16(clamped * 32767.0)
		binary.LittleEndian.PutUint16(buf[i*2:i*2+2], uint16(val))
	}

	_, err := w.Write(buf)
	return err
}

// ProcessWAV loads a source WAV file, renders it with style DSP filters, and writes output to dstPath.
func ProcessWAV(srcPath, dstPath string, style AcousticStyle) error {
	f, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer f.Close()

	samples, sampleRate, err := ReadWAV(f)
	if err != nil {
		return fmt.Errorf("read wav: %w", err)
	}

	rendered := RenderPCM(samples, sampleRate, style)

	tmpPath := fmt.Sprintf("%s.tmp.%d.%d", dstPath, os.Getpid(), time.Now().UnixNano())
	out, err := os.Create(tmpPath)
	if err != nil {
		return err
	}

	if err := WriteWAV(out, rendered, sampleRate); err != nil {
		_ = out.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("write wav: %w", err)
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}

	return os.Rename(tmpPath, dstPath)
}
