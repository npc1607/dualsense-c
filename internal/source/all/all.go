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
)
