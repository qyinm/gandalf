.PHONY: build test gate2 cross-machine-dogfood

BIN := bin/gandalf

build:
	@mkdir -p bin
	go build -o $(BIN) ./cmd/gandalf

test:
	go test ./...

gate2: build
	./scripts/gate2-acceptance.sh

cross-machine-dogfood: build
	./scripts/cross-machine-dogfood.sh
