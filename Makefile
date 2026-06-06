.PHONY: build test lint install clean

build:
	go build -o bin/meetup-pp-cli ./cmd/meetup-pp-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/meetup-pp-cli

clean:
	rm -rf bin/

build-mcp:
	go build -o bin/meetup-pp-mcp ./cmd/meetup-pp-mcp

install-mcp:
	go install ./cmd/meetup-pp-mcp

build-all: build build-mcp
