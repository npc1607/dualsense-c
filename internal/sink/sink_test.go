package sink

import (
	"context"
	"errors"
	"io"
	"log"
	"sync"
	"testing"
	"time"

	"github.com/npc1607/dualsense-c/internal/device"
	"github.com/npc1607/dualsense-c/internal/hid"
)

// fakeDevice is a controller stand-in whose writes can be made to fail.
type fakeDevice struct {
	info device.Info
	mu   sync.Mutex
	fail bool
}

func (f *fakeDevice) Info() device.Info { return f.info }
func (f *fakeDevice) Write([]byte) error {
	f.mu.Lock()
	fail := f.fail
	f.mu.Unlock()
	if fail {
		return errors.New("fake write failure")
	}
	return nil
}
func (f *fakeDevice) Read([]byte) (int, error) { return 0, io.EOF }
func (f *fakeDevice) Readable() bool           { return false }
func (f *fakeDevice) Close() error             { return nil }

func (f *fakeDevice) setFail(v bool) {
	f.mu.Lock()
	f.fail = v
	f.mu.Unlock()
}

func testSink(dev device.Device, rate int) *Sink {
	return New(dev, rate, log.New(io.Discard, "", 0))
}

func idleProducer(time.Time) hid.Frame { return hid.Idle() }

// TestStatsCountsWriteErrors drives a permanently failing device and checks the
// cumulative counter (not just the per-reattach log counter).
func TestStatsCountsWriteErrors(t *testing.T) {
	dev := &fakeDevice{info: device.Info{Path: "/dev/fake", Transport: device.USB}, fail: true}
	s := testSink(dev, 1000)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = s.Run(ctx, idleProducer)
	}()

	time.Sleep(30 * time.Millisecond)
	cancel()
	<-done

	if got := s.Stats().WriteErrors; got < 2 {
		t.Fatalf("write_errors = %d, want multiple failed ticks", got)
	}
	if got := s.Stats().Reconnects; got != 0 {
		t.Fatalf("reconnects = %d, want 0 without a reopen hook", got)
	}
}

// TestStatsCountsReconnects covers the recovery path: the first device fails,
// the hook returns a healthy one, and the counter records exactly one reattach.
func TestStatsCountsReconnects(t *testing.T) {
	bad := &fakeDevice{info: device.Info{Path: "/dev/fake0", Transport: device.USB}, fail: true}
	good := &fakeDevice{info: device.Info{Path: "/dev/fake1", Transport: device.USB}}
	s := testSink(bad, 1000)
	s.SetReopen(func(device.Device) (device.Device, error) {
		bad.setFail(false) // the replacement node works
		return good, nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = s.Run(ctx, idleProducer)
	}()

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if s.Stats().Reconnects == 1 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	cancel()
	<-done

	stats := s.Stats()
	if stats.Reconnects != 1 {
		t.Fatalf("reconnects = %d, want 1", stats.Reconnects)
	}
	if stats.WriteErrors < 1 {
		t.Fatalf("write_errors = %d, want at least the initial failure", stats.WriteErrors)
	}
}

// TestStatsConcurrentSnapshots is meaningful under -race: the sink goroutine
// writes counters while a CMD1-style reader samples them.
func TestStatsConcurrentSnapshots(t *testing.T) {
	dev := &fakeDevice{info: device.Info{Path: "/dev/fake", Transport: device.USB}}
	s := testSink(dev, 2000)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = s.Run(ctx, idleProducer)
	}()

	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				default:
					_ = s.Stats()
				}
			}
		}()
	}
	time.Sleep(20 * time.Millisecond)
	cancel()
	wg.Wait()
	<-done
}
