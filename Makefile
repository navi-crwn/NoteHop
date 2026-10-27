VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)

.PHONY: build test linux run clean

build:
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o bin/notehop ./cmd/notehop

test:
	go vet ./...
	go test -race ./...

# Static binaries to copy to a VPS (SQLite is pure Go, no cgo needed).
linux:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "$(LDFLAGS)" -o bin/notehop-linux-amd64 ./cmd/notehop
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags "$(LDFLAGS)" -o bin/notehop-linux-arm64 ./cmd/notehop

run:
	go run ./cmd/notehop -addr 127.0.0.1:8090 -db data/dev.db

clean:
	rm -rf bin
