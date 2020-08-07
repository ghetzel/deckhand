PKGS           := $(shell go list ./... 2> /dev/null | grep -v '/vendor')
LOCALS         := $(shell find . -type f -name '*.go' -not -path "./vendor*/*")
DECKHAND_BIN   ?= bin/deckhand

.EXPORT_ALL_VARIABLES:
GO111MODULE  = on

all: deps fmt build

fmt:
	gofmt -w $(LOCALS)
	go generate ./...
	go vet ./...
	go mod tidy

deps:
	go get ./...

test: fmt deps
	go test $(PKGS)

$(DECKHAND_BIN):
	go build --ldflags '-extldflags "-static"' -ldflags '-s' -o $(DECKHAND_BIN) *.go
	which deckhand && cp -v $(DECKHAND_BIN) $(shell which deckhand)

build: $(DECKHAND_BIN)

.PHONY: fmt deps build $(DECKHAND_BIN)