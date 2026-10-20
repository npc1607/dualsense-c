BIN     := bin
GO      := go
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w
GOFLAGS := -trimpath -ldflags "$(LDFLAGS) -X main.version=$(VERSION)"

.PHONY: all build build-onnx bridge windows test vet fmt ci clean install-udev

all: build bridge

build:
	$(GO) build $(GOFLAGS) -o $(BIN)/ds5d ./cmd/ds5d
	$(GO) build $(GOFLAGS) -o $(BIN)/ds5ctl ./cmd/ds5ctl
	$(GO) build $(GOFLAGS) -o $(BIN)/ds5vpad ./cmd/ds5vpad
	$(GO) build $(GOFLAGS) -o $(BIN)/ds5radio ./cmd/ds5radio
	$(GO) build $(GOFLAGS) -o $(BIN)/ffclient ./tools/ffclient

# Optional build with the in-process audio AI classifier (ONNX Runtime, CGO).
# Needs onnxruntime-cpu installed (libonnxruntime.so + headers) and the model:
#   models/yamnet.onnx, models/yamnet_class_map.csv  (~16 MB, not committed)
build-onnx:
	CGO_ENABLED=1 $(GO) build -tags onnx $(GOFLAGS) -o $(BIN)/ds5d-onnx ./cmd/ds5d

# The shared-memory bridge is a Windows binary: it runs inside a Proton prefix.
bridge:
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 $(GO) build $(GOFLAGS) -o $(BIN)/ds5bridge.exe ./cmd/ds5bridge

windows: bridge
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 $(GO) build $(GOFLAGS) -o $(BIN)/ds5d.exe ./cmd/ds5d
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 $(GO) build $(GOFLAGS) -o $(BIN)/ds5ctl.exe ./cmd/ds5ctl

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
