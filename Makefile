SHELL := /bin/bash
.DEFAULT_GOAL := help

BINARY  := bin/booking-api
PKG     := ./...
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

.PHONY: help
help: ## Show this help
	@grep -hE '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-14s\033[0m %s\n", $$1, $$2}'

.PHONY: build
build: ## Compile the service into ./bin
	@mkdir -p bin
	go build -trimpath \
		-ldflags="-s -w -X github.com/lemonbw/booking-api/internal/platform/version.Version=$(VERSION)" \
		-o $(BINARY) ./cmd/app

.PHONY: test
test: ## Run all unit tests with the race detector
	go test -race -count=1 $(PKG)

.PHONY: fmt
fmt: ## Format the code
	gofmt -s -w .

.PHONY: vet
vet: ## Run go vet
	go vet $(PKG)

.PHONY: up
up: ## Build and start the API + database
	docker compose build --build-arg VERSION=$(VERSION)
	docker compose up

.PHONY: down
down: ## Stop the stack
	docker compose down
