VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
GIT_COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

LDFLAGS := -X 'main.Version=$(VERSION)' \
           -X 'main.GitCommit=$(GIT_COMMIT)' \
           -X 'main.BuildDate=$(BUILD_DATE)'

all: build 

build:
	go build -ldflags "$(LDFLAGS)" -o secret_inject ./cmd/secret_inject

test:
	go test -v ./...

clean:
	rm -f secret_inject

install:
	go install -ldflags "$(LDFLAGS)" ./cmd/secret_inject

.PHONY: all build test clean install
