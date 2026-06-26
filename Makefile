.PHONY: build test gate2

BIN := bin/gandalf

build:
	@mkdir -p bin
	go build -o $(BIN) ./cmd/gandalf

test:
	go test ./...

gate2: build
	bun run dogfood:gate2