//go:build !onnx

package audioai

import "fmt"

// openEngine reports that this build has no inference backend.
//
// The default build stays pure Go (no cgo, cross-compiles everywhere), so the
// AI classifier is opt-in: rebuild with `-tags onnx` and the ONNX Runtime C
// library available.
func openEngine(modelPath string, threads int) (Engine, error) {
	return nil, fmt.Errorf("%w: model %q not loaded", ErrUnsupported, modelPath)
}
