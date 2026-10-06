BIN := bin/rockets-service
ROCKETS ?= ./rockets

.PHONY: run build test lint e2e tidy clean

run:
	go run ./cmd/rockets-service

build:
	go build -o $(BIN) ./cmd/rockets-service

test:
	go test -race ./...

lint:
	go vet ./...
	golangci-lint run ./...

e2e: build
	ROCKETS=$(ROCKETS) BIN=$(BIN) ./scripts/e2e.sh

tidy:
	go mod tidy

clean:
	rm -rf bin
