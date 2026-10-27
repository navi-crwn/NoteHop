.PHONY: run test build
run:
	go run ./cmd/notehop -addr 127.0.0.1:8090 -db data/dev.db
test:
	go test -race ./...
build:
	go build -trimpath -ldflags="-s -w" -o bin/notehop ./cmd/notehop
