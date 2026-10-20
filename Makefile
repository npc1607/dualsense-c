BIN     := bin
GO      := go
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w
GOFLAGS := -trimpath -ldflags "$(LDFLAGS) -X main.version=$(VERSION)"

ORT_WIN_DIR ?= /usr/x86_64-w64-mingw32
MINGW_CC    ?= x86_64-w64-mingw32-gcc

.PHONY: all build build-onnx build-windows-onnx bridge windows test vet fmt ci clean install-udev

all: build bridge

build:
	$(GO) build $(GOFLAGS) -o $(BIN)/ds5d ./cmd/ds5d
	$(GO) build $(GOFLAGS) -o $(BIN)/ds5ctl ./cmd/ds5ctl
	$(GO) build $(GOFLAGS) -o $(BIN)/ds5vpad ./cmd/ds5vpad
	$(GO) build $(GOFLAGS) -o $(BIN)/ds5radio ./cmd/ds5radio
	$(GO) build $(GOFLAGS) -o $(BIN)/ffclient ./tools/ffclient
	$(GO) build $(GOFLAGS) -o $(BIN)/hidprobe ./tools/hidprobe

# Optional build with the in-process audio AI classifier (ONNX Runtime, CGO).
# Needs onnxruntime-cpu installed (libonnxruntime.so + headers) and the model:
#   models/yamnet.onnx, models/yamnet_class_map.csv  (~16 MB, not committed)
build-onnx:
	CGO_ENABLED=1 $(GO) build -tags onnx $(GOFLAGS) -o $(BIN)/ds5d-onnx ./cmd/ds5d

# Optional Windows build with in-process ONNX Runtime (CGO + mingw-w64).
# Set ORT_WIN_DIR to the unpacked onnxruntime-win-x64 SDK directory containing
# include/ and lib/onnxruntime.lib (ship lib/onnxruntime.dll next to ds5d-onnx.exe).
build-windows-onnx:
	CGO_ENABLED=1 GOOS=windows GOARCH=amd64 CC=$(MINGW_CC) \
		CGO_CFLAGS="-I$(ORT_WIN_DIR)/include" \
		CGO_LDFLAGS="-L$(ORT_WIN_DIR)/lib" \
		$(GO) build -tags onnx $(GOFLAGS) -o $(BIN)/ds5d-onnx.exe ./cmd/ds5d

# The shared-memory bridge is a Windows binary: it runs inside a Proton prefix.
bridge:
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 $(GO) build $(GOFLAGS) -o $(BIN)/ds5bridge.exe ./cmd/ds5bridge

windows: bridge
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 $(GO) build $(GOFLAGS) -o $(BIN)/ds5d.exe ./cmd/ds5d
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 $(GO) build $(GOFLAGS) -o $(BIN)/ds5ctl.exe ./cmd/ds5ctl
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 $(GO) build $(GOFLAGS) -o $(BIN)/ds5vpad.exe ./cmd/ds5vpad
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 $(GO) build $(GOFLAGS) -o $(BIN)/ffclient.exe ./tools/ffclient
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 $(GO) build $(GOFLAGS) -o $(BIN)/hidprobe.exe ./tools/hidprobe

test:
	$(GO) test ./...

vet:
	$(GO) vet ./...
	GOOS=linux   $(GO) build ./...
	GOOS=windows $(GO) build ./...
	GOOS=darwin  $(GO) build ./...

fmt:
	gofmt -l -w .

ci: fmt vet test

install-udev:
	sudo cp udev/70-dualsense.rules udev/71-ds5vpad.rules /etc/udev/rules.d/
	sudo udevadm control --reload-rules
	sudo udevadm trigger
	@echo "now unplug and replug the controller"

clean:
	rm -rf $(BIN)
