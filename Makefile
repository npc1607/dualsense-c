BIN := bin
GO  := go

.PHONY: all build windows test vet fmt ci clean install-udev

all: build

build:
	$(GO) build -o $(BIN)/ds5d ./cmd/ds5d
	$(GO) build -o $(BIN)/ds5ctl ./cmd/ds5ctl

# Windows binaries cross-compile with pure Go (no cgo, no mingw).
windows:
	GOOS=windows GOARCH=amd64 $(GO) build -o $(BIN)/ds5d.exe ./cmd/ds5d
	GOOS=windows GOARCH=amd64 $(GO) build -o $(BIN)/ds5ctl.exe ./cmd/ds5ctl

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
	sudo cp udev/70-dualsense.rules /etc/udev/rules.d/
	sudo udevadm control --reload-rules
	sudo udevadm trigger
	@echo "now unplug and replug the controller"

clean:
	rm -rf $(BIN)
