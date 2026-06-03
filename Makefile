BINARY := azlocal
PKG    := github.com/yourusername/azlocal
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  := $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE    := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -s -w \
  -X main.version=$(VERSION) \
  -X main.commit=$(COMMIT) \
  -X main.date=$(DATE)

.PHONY: all build test vet fmt lint tidy run clean install

all: build

build:
	go build -ldflags '$(LDFLAGS)' -o bin/$(BINARY) ./cmd/azlocal

install:
	go install -ldflags '$(LDFLAGS)' ./cmd/azlocal

test:
	go test ./... -race -count=1

vet:
	go vet ./...

fmt:
	gofmt -s -w .

tidy:
	go mod tidy

run: build
	./bin/$(BINARY) $(ARGS)

clean:
	rm -rf bin dist .azlocal
