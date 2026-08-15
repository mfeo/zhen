BINARY  := zhen
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)

.PHONY: build test race vet fmt check install clean

build:
	go build -ldflags="$(LDFLAGS)" -o $(BINARY) .

test:
	go test ./...

race:
	go test -race -count=1 ./...

vet:
	go vet ./...

fmt:
	gofmt -w .

# What CI should run.
check: vet race

install:
	go install -ldflags="$(LDFLAGS)" .

clean:
	rm -f $(BINARY)
