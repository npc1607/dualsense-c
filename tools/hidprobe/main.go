// Command hidprobe writes DualSense output reports with different flag/volume
// combinations while audio is being played to the controller's 4-channel USB
// interface. It exists to answer one hardware question that no amount of unit
// testing can: which report configuration makes the voice coils (audio
// channels 3/4) actually act.
//
// Each variant is announced with N rumble pulses, so a tester can report
// "variant N vibrated" without watching a screen.
//
//	./bin/hidprobe                            # auto-detect DualSense, variants 1..4
//	./bin/hidprobe -dev /dev/hidraw6          # explicit path
//	./bin/hidprobe -dev /dev/hidraw6 -only 3  # just one
//
// Play low-frequency audio into the controller at the same time, e.g.
//
//	pacat --playback --device=<ds5 sink> --rate=48000 --channels=4 \
//	      --channel-map=front-left,front-right,rear-left,rear-right \
//	      --format=s16le --raw /tmp/coil4.raw
package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/npc1607/dualsense-c/internal/device"
)

// Report offsets (same layout the project uses for USB reports).
const (
	offFlags0  = 1
	offFlags1  = 2
	offRumbleR = 3
	offRumbleL = 4
	offPhone   = 5 // headphone volume (only meaningful when bit 4 of flags0 is set)
	offSpeaker = 6 // speaker volume
	offMic     = 7 // microphone volume
	offAudio   = 8 // audio control
	offValid2  = 39
	offBright  = 43
	offLight   = 45
)

type variant struct {
	name string
	size int
	f0   byte
	f1   byte
	f2   byte
	// audio fields, only written when fillAudio is set
	fillAudio   bool
	phone       byte
	speaker     byte
	mic         byte
	audioCtl    byte
	description string
}

var variants = []variant{
	{
		name: "A", size: 48, f0: 0x0F, f1: 0x57, f2: 0x03,
		description: "48 字节报告 + flags0=0x0F（当前 ds5d 的行为，已知 Ch1/2 有声）",
	},
	{
		name: "B", size: 63, f0: 0x0F, f1: 0x57, f2: 0x03,
		description: "63 字节报告（内核 hid-playstation 的实际长度）",
	},
	{
		name: "C", size: 63, f0: 0x0F, f1: 0x57, f2: 0x07,
		description: "63 字节 + flags2|=0x04",
	},
	{
		name: "D", size: 63, f0: 0xFF, f1: 0x57, f2: 0x07,
		fillAudio: true, phone: 0x7F, speaker: 0x7F, mic: 0x40, audioCtl: 0x00,
		description: "63 字节 + 启用音频字段并写入最大音量",
	},
}

func main() {
	dev := flag.String("dev", "", "DualSense device path (default: auto-detect)")
	only := flag.Int("only", 0, "run only this variant number (1-based), 0 = all")
	hold := flag.Duration("hold", 3*time.Second, "how long each variant runs")
	noMarker := flag.Bool("no-marker", false, "skip the numbered rumble pulses (use when testing the coils, which the markers would mask)")
	flag.Parse()

	var (
		d   device.Device
		err error
	)
	if *dev != "" {
		d, err = device.OpenPath(*dev)
	} else {
		var info device.Info
		info, err = device.Find()
		if err == nil {
			d, err = device.Open(info)
		}
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "open device: %v\n", err)
		os.Exit(1)
	}
	defer d.Close()

	fmt.Printf("device: %s\n", d.Info())
	fmt.Println("请同时往手柄 Ch3/4（音圈）播放 40Hz 低频；每个变体前面有 N 次震动作为编号。")
	for i, v := range variants {
		n := i + 1
		if *only != 0 && *only != n {
			continue
		}
		fmt.Printf("\n=== 变体 %s: %s\n", v.name, v.description)
		if !*noMarker {
			fmt.Printf("    (接下来 %d 次震动是编号标记)\n", n)
			marker(d, n)
		}
		fmt.Printf("    >>> 现在开始纯音圈测试（无标记震动），持续 %s\n", *hold)
		tick(d, v, *hold)
	}
	fmt.Println("\n完成。")
}

// marker pulses the motors n times so the tester can identify the variant.
func marker(d device.Device, n int) {
	b := make([]byte, 64)
	b[0] = 0x02
	b[offFlags0] = 0x0F
	b[offFlags1] = 0x57
	b[offRumbleR] = 220
	b[offRumbleL] = 220
	b[offValid2] = 0x03
	b[offBright] = 0x02
	for i := 0; i < n; i++ {
		_ = d.Write(b[:48])
		time.Sleep(140 * time.Millisecond)
		b[offRumbleR], b[offRumbleL] = 0, 0
		_ = d.Write(b[:48])
		time.Sleep(140 * time.Millisecond)
	}
}

// tick streams the variant's report at 125 Hz for dur, with motors silent.
func tick(d device.Device, v variant, dur time.Duration) {
	b := make([]byte, v.size)
	b[0] = 0x02
	b[offFlags0] = v.f0
	b[offFlags1] = v.f1
	if v.fillAudio {
		b[offPhone] = v.phone
		b[offSpeaker] = v.speaker
		b[offMic] = v.mic
		b[offAudio] = v.audioCtl
	}
	b[offValid2] = v.f2
	b[offBright] = 0x02
	if v.size > offLight+3 {
		// lightbar: dim white, so the report is visibly accepted
		b[offLight], b[offLight+1], b[offLight+2] = 30, 30, 30
	}
	deadline := time.Now().Add(dur)
	for time.Now().Before(deadline) {
		if err := d.Write(b); err != nil {
			fmt.Fprintf(os.Stderr, "write: %v\n", err)
			return
		}
		time.Sleep(8 * time.Millisecond)
	}
}
