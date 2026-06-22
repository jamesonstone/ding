.PHONY: build test lint fmt vet clean tidy help

BINARY_NAME=ding

build: ## build the ding binary into bin/
	go build -o bin/$(BINARY_NAME) ./cmd/ding

test: ## run all tests
	go test ./...

lint: ## run golangci-lint (if installed)
	golangci-lint run ./...

fmt: ## format all Go source
	go fmt ./...

vet: ## run go vet
	go vet ./...

clean: ## remove build artifacts
	rm -rf bin/
	go clean

tidy: ## tidy go.mod / go.sum
	go mod tidy

help: ## list available targets
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN{FS=":.*?## "}{printf "  %-10s %s\n", $$1, $$2}'

all: fmt vet test build
