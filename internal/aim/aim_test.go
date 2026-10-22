package aim_test

import (
	"math"
	"testing"
	"time"

	"github.com/npc1607/dualsense-c/internal/aim"
)

func TestCurves(t *testing.T) {
	// Test Linear
	linear := aim.NewCurve(aim.CurveConfig{
		Type:          aim.CurveLinear,
		InnerDeadzone: 0.05,
	})
	// Inside deadzone should be 0
	ox, oy := linear.TransformStick(0.02, 0.03)
	if ox != 0 || oy != 0 {
		t.Fatalf("expected deadzone zeroing, got (%f, %f)", ox, oy)
	}
	// At full scale
	ox, oy = linear.TransformStick(1.0, 0)
	if math.Abs(float64(ox)-1.0) > 1e-3 || oy != 0 {
		t.Fatalf("expected (1.0, 0), got (%f, %f)", ox, oy)
	}

	// Test Sigmoid dual-normalization
	sig := aim.NewCurve(aim.CurveConfig{
		Type:          aim.CurveSigmoid,
		Exponent:      6.5,
		MidPoint:      0.52,
		InnerDeadzone: 0.04,
		OuterBoost:    1.5,
	})
	// S(0) must equal 0
	if s0 := sig.Transform(0.0); math.Abs(s0) > 1e-6 {
		t.Fatalf("Sigmoid S(0) must be 0, got %f", s0)
	}
	// S(1) must equal 1.0 (without outer boost at r=1.0 boost is evaluated in Transform, wait: Transform clamps to 1.0)
	if s1 := sig.Transform(1.0); math.Abs(s1-1.0) > 1e-6 {
		t.Fatalf("Sigmoid S(1) must be 1, got %f", s1)
	}
	// Monotonicity check
	prev := 0.0
	for r := 0.05; r <= 1.0; r += 0.05 {
		val := sig.Transform(r)
		if val < prev {
			t.Fatalf("Sigmoid non-monotonic at %f: %f < %f", r, val, prev)
		}
		prev = val
	}

	// Test Power curve
	pwr := aim.NewCurve(aim.CurveConfig{
		Type:     aim.CurvePower,
		Exponent: 2.0,
	})
	pMid := pwr.Transform(0.5)
	if math.Abs(pMid-0.25) > 1e-4 {
		t.Fatalf("Power(0.5) with gamma=2 expected 0.25, got %f", pMid)
	}

	// Test DualZone
	dz := aim.NewCurve(aim.CurveConfig{
		Type:     aim.CurveDualZone,
		MidPoint: 0.85,
		Exponent: 0.35, // alpha
	})
	dzSplit := dz.Transform(0.85)
	if math.Abs(dzSplit-0.35) > 1e-4 {
		t.Fatalf("DualZone at split 0.85 expected 0.35, got %f", dzSplit)
	}
}

func TestOneEuroFilter(t *testing.T) {
	filt := aim.NewOneEuroFilter(0.85, 0.045, 1.0)

	// Low-speed jitter test: alternating +/- 0.01 around 0.5 at 125Hz (dt=0.008)
	var jitterFiltered []float64
	for i := 0; i < 20; i++ {
		val := 0.5
		if i%2 == 0 {
			val += 0.01
		} else {
			val -= 0.01
		}
		out := filt.Filter(val, 0.008)
		jitterFiltered = append(jitterFiltered, out)
	}
	// After settling, variation in jitterFiltered should be much smaller than 0.02
	last := jitterFiltered[len(jitterFiltered)-1]
	prev := jitterFiltered[len(jitterFiltered)-2]
	if math.Abs(last-prev) >= 0.02 {
		t.Fatalf("OneEuro did not attenuate low-speed jitter: delta=%f", math.Abs(last-prev))
	}

	// High-speed step response convergence
	filt.Reset()
	_ = filt.Filter(0.0, 0.008)
	var stepOut float64
	for i := 0; i < 25; i++ {
		stepOut = filt.Filter(1.0, 0.008)
	}
	if stepOut < 0.8 {
		t.Fatalf("OneEuro did not converge to step input after 25 frames, got %f", stepOut)
	}
}

func TestGyroProjection(t *testing.T) {
	mapper := aim.NewGyroMapper(aim.GyroConfig{
		Enabled:          true,
		Trigger:          aim.GyroTriggerL2Hold,
		TriggerThreshold: 0.15,
		NaturalAngleDeg:  35.0,
		DeadzoneDPS:      5.0,
		SensYaw:          1.85,
		SensPitch:        1.35,
		InvertPitch:      true,
	})

	// Inactive when L2 < 0.15
	dx, dy := mapper.Process(0.1, 0.1, 0.0, 0.10, false)
	if dx != 0 || dy != 0 {
		t.Fatalf("expected 0, 0 when trigger not pressed, got %f, %f", dx, dy)
	}

	// Active when L2 >= 0.15, but deadzone check: small angular velocity < 5 DPS
	// 5 DPS / 2000 = 0.0025 full scale
	dx, dy = mapper.Process(0.001, 0.001, 0.0, 0.5, false)
	if dx != 0 || dy != 0 {
		t.Fatalf("expected deadzone suppression, got %f, %f", dx, dy)
	}

	// Pure pitch with InvertPitch
	_, dy = mapper.Process(0, 0.1, 0, 0.5, false)
	if dy >= 0 {
		t.Fatalf("expected negative dy due to InvertPitch, got %f", dy)
	}

	// Verify grip rotation projection
	// Angle 35 deg: EffYaw = Yaw*cos(35) - Roll*sin(35)
	rad := 35.0 * math.Pi / 180.0
	expectedEff := 0.2*math.Cos(rad) - 0.1*math.Sin(rad)
	expectedDX := float32(expectedEff * 1.85)

	dx, _ = mapper.Process(0.2, 0, 0.1, 0.5, false)
	if math.Abs(float64(dx-expectedDX)) > 1e-4 {
		t.Fatalf("expected dx %f, got %f", expectedDX, dx)
	}
}

func TestAntiRecoil(t *testing.T) {
	recoil := aim.NewAntiRecoil(aim.AntiRecoilConfig{
		Enabled:      true,
		Trigger:      "r2_full",
		StartDelay:   50 * time.Millisecond,
		BurstForce:   400,
		BurstDecay:   0.02,
		SustainForce: 150,
		JitterNoise:  0, // 0 for deterministic testing
	})

	t0 := time.Now()
	// T0: R2 pulled
	d0 := recoil.Process(0.9, t0)
	if d0 != 0 {
		t.Fatalf("expected 0 recoil at start edge, got %f", d0)
	}

	// Inside start delay (30ms < 50ms)
	d30 := recoil.Process(0.9, t0.Add(30*time.Millisecond))
	if d30 != 0 {
		t.Fatalf("expected 0 recoil during start delay, got %f", d30)
	}

	// Immediately after start delay (50ms)
	// Force = 400 * exp(0) + 150 = 550. Delta = 550 / 32768 = 0.01678
	d50 := recoil.Process(0.9, t0.Add(50*time.Millisecond))
	if math.Abs(float64(d50)-(550.0/32768.0)) > 1e-4 {
		t.Fatalf("expected peak burst recoil at 50ms, got %f", d50)
	}

	// Much later (500ms -> tEff = 450ms)
	// exp(-0.02 * 450) = exp(-9.0) ~ 0. Force ~= 150. Delta = 150 / 32768 = 0.004577
	d500 := recoil.Process(0.9, t0.Add(500*time.Millisecond))
	expectedSustain := float32(150.0 / 32768.0)
	if math.Abs(float64(d500-expectedSustain)) > 5e-4 {
		t.Fatalf("expected sustain force ~%f, got %f", expectedSustain, d500)
	}

	// Releasing R2 resets recoil
	dRelease := recoil.Process(0.1, t0.Add(600*time.Millisecond))
	if dRelease != 0 {
		t.Fatalf("expected 0 recoil when released, got %f", dRelease)
	}
}

func TestCompositeProcessor(t *testing.T) {
	proc := aim.NewProcessor(
		aim.CurveConfig{
			Type:          aim.CurveSigmoid,
			Exponent:      6.5,
			MidPoint:      0.52,
			InnerDeadzone: 0.04,
			OuterBoost:    1.5,
		},
		aim.FilterConfig{
			Type:      aim.FilterOneEuro,
			MinCutoff: 0.85,
			Beta:      0.045,
			DCutoff:   1.0,
		},
		aim.GyroConfig{
			Enabled:          true,
			Trigger:          aim.GyroTriggerL2Hold,
			TriggerThreshold: 0.15,
			NaturalAngleDeg:  35.0,
			DeadzoneDPS:      0.4,
			SensYaw:          1.85,
			SensPitch:        1.35,
		},
		aim.AntiRecoilConfig{
			Enabled:      true,
			Trigger:      "r2_full",
			StartDelay:   60 * time.Millisecond,
			BurstForce:   420,
			BurstDecay:   0.015,
			SustainForce: 180,
			JitterNoise:  25,
		},
	)

	now := time.Now()
	// Simulate 250Hz input stream for 1 second (250 frames)
	for i := 0; i < 250; i++ {
		tFrame := now.Add(time.Duration(i*4) * time.Millisecond)
		rx := float32(0.5 * math.Sin(float64(i)*0.1))
		ry := float32(0.5 * math.Cos(float64(i)*0.1))

		ox, oy := proc.Process(aim.InputFrame{
			RX:          rx,
			RY:          ry,
			GyroYaw:     0.02,
			GyroPitch:   -0.01,
			GyroRoll:    0.005,
			L2:          0.8,
			R2:          0.9,
			TouchActive: false,
			At:          tFrame,
		})

		if math.IsNaN(float64(ox)) || math.IsInf(float64(ox), 0) {
			t.Fatalf("frame %d produced invalid ox: %f", i, ox)
		}
		if math.IsNaN(float64(oy)) || math.IsInf(float64(oy), 0) {
			t.Fatalf("frame %d produced invalid oy: %f", i, oy)
		}
		if ox < -1.0 || ox > 1.0 {
			t.Fatalf("frame %d ox clamped out of range: %f", i, ox)
		}
		if oy < -1.0 || oy > 1.0 {
			t.Fatalf("frame %d oy clamped out of range: %f", i, oy)
		}
	}
}

func TestFilterEMAAndReset(t *testing.T) {
	sf := aim.NewStickFilter(aim.FilterConfig{Type: aim.FilterEMA})
	x, y := sf.Filter(1.0, -1.0, 8*time.Millisecond)
	if x != 1.0 || y != -1.0 {
		t.Fatalf("EMA first sample should match input, got %f, %f", x, y)
	}
	x, y = sf.Filter(0.0, 0.0, 8*time.Millisecond)
	if x >= 1.0 || y <= -1.0 {
		t.Fatalf("EMA second sample should smooth towards 0, got %f, %f", x, y)
	}
	sf.Reset()

	sfNone := aim.NewStickFilter(aim.FilterConfig{Type: aim.FilterNone})
	nx, ny := sfNone.Filter(0.5, -0.5, 8*time.Millisecond)
	if nx != 0.5 || ny != -0.5 {
		t.Fatalf("FilterNone should return untouched, got %f, %f", nx, ny)
	}
}

func TestProcessorReset(t *testing.T) {
	proc := aim.NewProcessor(
		aim.CurveConfig{Type: aim.CurveLinear},
		aim.FilterConfig{Type: aim.FilterNone},
		aim.GyroConfig{},
		aim.AntiRecoilConfig{},
	)
	proc.Reset()
}

func TestGyroTriggers(t *testing.T) {
	mTouch := aim.NewGyroMapper(aim.GyroConfig{
		Enabled: true,
		Trigger: aim.GyroTriggerTouch,
	})
	if mTouch.IsActive(0, false) {
		t.Fatal("touch trigger should be inactive when touchActive is false")
	}
	if !mTouch.IsActive(0, true) {
		t.Fatal("touch trigger should be active when touchActive is true")
	}

	mAlways := aim.NewGyroMapper(aim.GyroConfig{
		Enabled: true,
		Trigger: aim.GyroTriggerAlways,
	})
	if !mAlways.IsActive(0, false) {
		t.Fatal("always trigger should be active")
	}
}
