.PHONY: build test restore-safety gate2 changes-home cross-machine-dogfood

BIN := bin/gandalf

build:
	@mkdir -p bin
	go build -o $(BIN) ./cmd/gandalf

test:
	go test -count=1 ./...

restore-safety: build
	./scripts/restore-safety-regression.sh

gate2: build
	./scripts/gate2-console-acceptance.sh

changes-home: build
	./scripts/changes-home-acceptance.sh

cross-machine-dogfood: build
	./scripts/cross-machine-dogfood.sh
