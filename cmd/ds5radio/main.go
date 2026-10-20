// Command ds5radio is the race engineer companion broadcaster for DualSense.
// It delivers team radio calls, pacenotes, and spotter warnings through the
// controller's built-in front speaker (audio channels 1 & 2: FL/FR) while
// the haptic engine continues driving the voice coils on channels 3 & 4.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	ossignal "os/signal"
	"syscall"
	"time"

	"github.com/npc1607/dualsense-c/internal/radio"
)

var version = "dev"

func main() {
	var (
		sinkName    = flag.String("sink", "", "override audio sink name (default: auto-detect DualSense)")
		clipsDir    = flag.String("clips", "", "path to directory with .wav radio clips")
		listenAddr  = flag.String("listen", "127.0.0.1:9999", "UDP telemetry address to listen on")
		decoderName = flag.String("decoder", "acshm", "telemetry decoder (acshm, forza, dirt, outgauge)")
		showVer     = flag.Bool("version", false, "print version and exit")
	)
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: ds5radio [flags] <command> [args...]\n\n")
		fmt.Fprintf(os.Stderr, "Commands:\n")
		fmt.Fprintf(os.Stderr, "  test             Play sample F1/Rally engineer radio clips on the controller speaker\n")
		fmt.Fprintf(os.Stderr, "  play <clip>      Play a specific radio clip by name (e.g. tire_slip, engine_redline)\n")
		fmt.Fprintf(os.Stderr, "  list             List all available radio audio clips in assets/radio\n")
		fmt.Fprintf(os.Stderr, "  listen           Run the live telemetry monitor and broadcast radio calls automatically\n\n")
		fmt.Fprintf(os.Stderr, "Flags:\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	if *showVer {
		fmt.Printf("ds5radio %s\n", version)
		return
	}

	args := flag.Args()
	if len(args) == 0 {
		flag.Usage()
		os.Exit(1)
	}

	logger := log.New(os.Stderr, "ds5radio: ", log.LstdFlags)

	player, err := radio.NewPlayer(radio.Config{
		SinkName: *sinkName,
		ClipsDir: *clipsDir,
		Logger:   logger,
	})
	if err != nil {
		logger.Fatalf("init player: %v", err)
	}

	ctx, cancel := ossignal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	switch args[0] {
	case "test":
		runTest(ctx, player, logger)
	case "play":
		if len(args) < 2 {
			logger.Fatalf("usage: ds5radio play <clip-name>")
		}
		clip := args[1]
		logger.Printf("playing clip %q through DualSense speaker...", clip)
		if err := player.Play(ctx, clip); err != nil {
			logger.Fatalf("play %q: %v", clip, err)
		}
		logger.Printf("done.")
	case "list":
		clips, err := player.ListClips()
		if err != nil {
			logger.Fatalf("list clips: %v", err)
		}
		fmt.Printf("Available radio clips (%d):\n", len(clips))
		for _, c := range clips {
			fmt.Printf("  - %s\n", c)
		}
	case "listen":
		monitor := radio.NewMonitor(player, logger)
		logger.Printf("starting radio monitor on %s (decoder: %s)...", *listenAddr, *decoderName)
		if err := monitor.Run(ctx, *listenAddr, *decoderName); err != nil {
			logger.Fatalf("monitor: %v", err)
		}
	default:
		logger.Fatalf("unknown command %q (see ds5radio -h)", args[0])
	}
}

func runTest(ctx context.Context, player *radio.Player, logger *log.Logger) {
	testClips := []string{"tire_slip", "engine_redline", "fastest_pace", "f1_box"}
	logger.Printf("=== DualSense 赛车电台扬声器试听测试 ===")
	logger.Printf("准备依次播放 4 段经过电台带通滤波与对讲机 Beep 特效处理的工程师语音...")

	for i, clip := range testClips {
		if ctx.Err() != nil {
			break
		}
		logger.Printf("[%d/%d] 正在播放: %s ...", i+1, len(testClips), clip)
		if err := player.Play(ctx, clip); err != nil {
			logger.Printf("播放失败: %v", err)
		}
		time.Sleep(1 * time.Second)
	}
	logger.Printf("测试播放完成！")
}
