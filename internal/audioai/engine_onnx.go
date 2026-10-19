//go:build onnx

// ONNX Runtime backend for the audio classifier.
//
// It links the ONNX Runtime C API directly: no Python, no cgo in the default
// build (this file is behind the `onnx` build tag).
//
// Linux:   sudo pacman -S onnxruntime-cpu   (or the distro equivalent)
// Windows: same source, link onnxruntime.lib and ship onnxruntime.dll;
//
//	cross-compiling needs CGO + mingw-w64.
//
// Build: go build -tags onnx ./cmd/ds5d
package audioai

import (
	"fmt"
	"runtime"
	"unsafe"
)

/*
#cgo CFLAGS: -I/usr/include/onnxruntime -I/usr/include
#cgo linux LDFLAGS: -lonnxruntime
#cgo windows LDFLAGS: -lonnxruntime

#include <stdlib.h>
#include <onnxruntime_c_api.h>

static const OrtApi* ds5_api(void) {
	const OrtApiBase* b = OrtGetApiBase();
	return b->GetApi(ORT_API_VERSION);
}

static const char* ds5_err(const OrtApi* a, OrtStatus* st) {
	if (st == NULL) return "ok";
	const char* m = a->GetErrorMessage(st);
	return m ? m : "unknown";
}

static OrtStatus* ds5_create_env(const OrtApi* a, OrtEnv** env) {
	return a->CreateEnv(ORT_LOGGING_LEVEL_WARNING, "ds5d", env);
}

static OrtStatus* ds5_meminfo(const OrtApi* a, OrtMemoryInfo** mi) {
	return a->CreateCpuMemoryInfo(OrtArenaAllocator, OrtMemTypeDefault, mi);
}

static OrtStatus* ds5_session_options(const OrtApi* a, int threads, OrtSessionOptions** opts) {
	OrtStatus* st = a->CreateSessionOptions(opts);
	if (st) return st;
	st = a->SetSessionGraphOptimizationLevel(*opts, ORT_ENABLE_ALL);
	if (st) return st;
	if (threads > 0) {
		st = a->SetIntraOpNumThreads(*opts, threads);
	}
	return st;
}

static OrtStatus* ds5_create_session(const OrtApi* a, OrtEnv* env, const char* path,
		OrtSessionOptions* opts, OrtSession** sess) {
	return a->CreateSession(env, path, opts, sess);
}

static OrtStatus* ds5_input_name(const OrtApi* a, OrtSession* sess, char** name) {
	OrtAllocator* alloc = NULL;
	OrtStatus* st = a->GetAllocatorWithDefaultOptions(&alloc);
	if (st) return st;
	return a->SessionGetInputName(sess, 0, alloc, name);
}

static OrtStatus* ds5_output_count(const OrtApi* a, OrtSession* sess, size_t* n) {
	return a->SessionGetOutputCount(sess, n);
}

static OrtStatus* ds5_output_name(const OrtApi* a, OrtSession* sess, size_t i, char** name) {
	OrtAllocator* alloc = NULL;
	OrtStatus* st = a->GetAllocatorWithDefaultOptions(&alloc);
	if (st) return st;
	return a->SessionGetOutputName(sess, i, alloc, name);
}

static OrtStatus* ds5_tensor(const OrtApi* a, OrtMemoryInfo* mi, float* data, size_t n,
		const int64_t* shape, size_t dims, OrtValue** v) {
	return a->CreateTensorWithDataAsOrtValue(mi, data, n * sizeof(float), shape, dims,
		ONNX_TENSOR_ELEMENT_DATA_TYPE_FLOAT, v);
}

static OrtStatus* ds5_run(const OrtApi* a, OrtSession* sess, OrtValue* in, const char* in_name,
		const char* out_name, OrtValue** out) {
	const char* in_names[1];
	const char* out_names[1];
	in_names[0] = in_name;
	out_names[0] = out_name;
	// Outputs must be requested by name; NULL names + count 0 is rejected.
	return a->Run(sess, NULL, in_names, (const OrtValue* const*)&in, 1, out_names, 1, out);
}

static OrtStatus* ds5_dims(const OrtApi* a, OrtValue* v, size_t* count, int64_t* dims, size_t max) {
	OrtTensorTypeAndShapeInfo* info = NULL;
	OrtStatus* st = a->GetTensorTypeAndShape(v, &info);
	if (st) return st;
	st = a->GetDimensionsCount(info, count);
	if (st) return st;
	if (*count > max) {
		*count = max;
	}
	st = a->GetDimensions(info, dims, *count);
	a->ReleaseTensorTypeAndShapeInfo(info);
	return st;
}

static OrtStatus* ds5_data(const OrtApi* a, OrtValue* v, float** out, size_t* n) {
	return a->GetTensorMutableData(v, (void**)out);
}

// cgo cannot call function pointers, so every OrtApi member we use gets a wrapper.
static void ds5_release_status(const OrtApi* a, OrtStatus* st) { a->ReleaseStatus(st); }
static void ds5_release_value(const OrtApi* a, OrtValue* v) { a->ReleaseValue(v); }
static void ds5_release_session(const OrtApi* a, OrtSession* s) { a->ReleaseSession(s); }
static void ds5_release_session_options(const OrtApi* a, OrtSessionOptions* o) { a->ReleaseSessionOptions(o); }
static void ds5_release_memory_info(const OrtApi* a, OrtMemoryInfo* m) { a->ReleaseMemoryInfo(m); }
static void ds5_release_env(const OrtApi* a, OrtEnv* e) { a->ReleaseEnv(e); }
*/
import "C"

// ortEngine holds the ONNX Runtime session for one model.
type ortEngine struct {
	api    *C.OrtApi
	env    *C.OrtEnv
	mem    *C.OrtMemoryInfo
	opts   *C.OrtSessionOptions
	sess   *C.OrtSession
	input  *C.char // session-owned, not freed
	output *C.char // session-owned, not freed

	outCount C.size_t
}

func openEngine(modelPath string, threads int) (Engine, error) {
	api := C.ds5_api()
	if api == nil {
		return nil, fmt.Errorf("audioai: ONNX Runtime version mismatch (lib built against a different API version)")
	}
	e := &ortEngine{api: api}

	if st := C.ds5_create_env(api, &e.env); st != nil {
		return nil, e.wrap("CreateEnv", st)
	}
	if st := C.ds5_meminfo(api, &e.mem); st != nil {
		return nil, e.wrap("CreateCpuMemoryInfo", st)
	}
	if st := C.ds5_session_options(api, C.int(threads), &e.opts); st != nil {
		return nil, e.wrap("CreateSessionOptions", st)
	}
	cPath := C.CString(modelPath)
	defer C.free(unsafe.Pointer(cPath))
	if st := C.ds5_create_session(api, e.env, cPath, e.opts, &e.sess); st != nil {
		return nil, e.wrap("CreateSession", st)
	}
	if st := C.ds5_input_name(api, e.sess, &e.input); st != nil {
		return nil, e.wrap("SessionGetInputName", st)
	}
	if st := C.ds5_output_count(api, e.sess, &e.outCount); st != nil {
		return nil, e.wrap("SessionGetOutputCount", st)
	}
	if e.outCount == 0 {
		return nil, fmt.Errorf("audioai: model has no outputs")
	}
	if st := C.ds5_output_name(api, e.sess, 0, &e.output); st != nil {
		return nil, e.wrap("SessionGetOutputName", st)
	}
	return e, nil
}

func (e *ortEngine) wrap(op string, st *C.OrtStatus) error {
	msg := C.GoString(C.ds5_err(e.api, st))
	C.ds5_release_status(e.api, st)
	return fmt.Errorf("audioai: %s: %s", op, msg)
}

// Classify feeds one 16 kHz window and returns the first output (scores),
// averaging over frames when the model returns [frames, classes].
func (e *ortEngine) Classify(pcm []float32) ([]float32, error) {
	if len(pcm) == 0 {
		return nil, fmt.Errorf("audioai: empty window")
	}
	n := C.size_t(len(pcm))
	// This YAMNet export takes a rank-1 waveform ([samples]); a [1, samples]
	// tensor is rejected with "Invalid rank for input: waveform".
	shape := [1]C.int64_t{C.int64_t(len(pcm))}

	var in *C.OrtValue
	if st := C.ds5_tensor(e.api, e.mem, (*C.float)(unsafe.Pointer(&pcm[0])), n,
		&shape[0], 1, &in); st != nil {
		return nil, e.wrap("CreateTensorWithDataAsOrtValue", st)
	}
	defer C.ds5_release_value(e.api, in)

	outs := make([]*C.OrtValue, 1)
	if st := C.ds5_run(e.api, e.sess, in, e.input, e.output, &outs[0]); st != nil {
		return nil, e.wrap("Run", st)
	}
	defer func() {
		for _, o := range outs {
			if o != nil {
				C.ds5_release_value(e.api, o)
			}
		}
	}()

	var dimsCount C.size_t
	dims := make([]C.int64_t, 4)
	if st := C.ds5_dims(e.api, outs[0], &dimsCount, &dims[0], 4); st != nil {
		return nil, e.wrap("GetTensorShape", st)
	}

	var data *C.float
	if st := C.ds5_data(e.api, outs[0], &data, nil); st != nil {
		return nil, e.wrap("GetTensorMutableData", st)
	}

	total := int64(1)
	for i := 0; i < int(dimsCount); i++ {
		total *= int64(dims[i])
	}
	if total <= 0 {
		return nil, fmt.Errorf("audioai: model returned an empty tensor")
	}
	scores := unsafe.Slice((*float32)(unsafe.Pointer(data)), int(total))

	// [frames, classes] -> average over frames. [classes] -> as is.
	classes := len(scores)
	frames := 1
	if int(dimsCount) >= 2 {
		classes = int(dims[dimsCount-1])
		frames = len(scores) / classes
	}
	if classes <= 0 || frames <= 0 {
		return nil, fmt.Errorf("audioai: unexpected output shape (dims=%d)", int(dimsCount))
	}
	out := make([]float32, classes)
	for f := 0; f < frames; f++ {
		row := scores[f*classes : (f+1)*classes]
		for c := 0; c < classes; c++ {
			out[c] += row[c]
		}
	}
	for c := range out {
		out[c] /= float32(frames)
	}
	return out, nil
}

// Close releases the runtime objects. Safe to call twice.
func (e *ortEngine) Close() error {
	if e.api == nil {
		return nil
	}
	if e.sess != nil {
		C.ds5_release_session(e.api, e.sess)
		e.sess = nil
	}
	if e.opts != nil {
		C.ds5_release_session_options(e.api, e.opts)
		e.opts = nil
	}
	if e.mem != nil {
		C.ds5_release_memory_info(e.api, e.mem)
		e.mem = nil
	}
	if e.env != nil {
		C.ds5_release_env(e.api, e.env)
		e.env = nil
	}
	e.api = nil
	runtime.KeepAlive(e)
	return nil
}
