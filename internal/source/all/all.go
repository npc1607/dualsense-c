package all

import (
	// device-free scripted source (profile development, tests)
	_ "github.com/npc1607/dualsense-c/internal/source/sim"

	// input-driven source (any game)
	_ "github.com/npc1607/dualsense-c/internal/source/trigger"

	// native game telemetry over UDP (Forza, BeamNG, DiRT family, ...)
	_ "github.com/npc1607/dualsense-c/internal/source/udp"

	// audio envelope (bass/mid/loudness) from a PipeWire monitor
	_ "github.com/npc1607/dualsense-c/internal/source/audio"

	// force feedback captured by a virtual gamepad (ds5vpad)
	_ "github.com/npc1607/dualsense-c/internal/source/ffb"

	// several sources at once (e.g. trigger + audio for shooters)
	_ "github.com/npc1607/dualsense-c/internal/source/combo"

	// audio AI event detector source (local YAMNet/ONNX loopback)
	_ "github.com/npc1607/dualsense-c/internal/source/audioai"
)
