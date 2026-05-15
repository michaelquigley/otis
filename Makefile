TARGETS ?= ./cmd/otis

.PHONY: build test

build:
	go install $(TARGETS)

test:
	go test ./... -count=1
	go vet ./...
