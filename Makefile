.PHONY: build test gate2

BIN := bin/hem

build:
	@mkdir -p bin
	go build -o $(BIN) ./cmd/hem

test:
	go test ./...

gate2: build
	bun run dogfood:gate2