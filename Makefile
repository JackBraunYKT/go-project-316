.PHONY: build test run

GOCACHE_DIR := $(CURDIR)/.cache/go-build
GO := GOCACHE=$(GOCACHE_DIR) go

build:
	@mkdir -p bin $(GOCACHE_DIR)
	@$(GO) build -o bin/hexlet-go-crawler ./cmd/hexlet-go-crawler

test:
	@mkdir -p $(GOCACHE_DIR)
	@$(GO) test ./...

run:
	@mkdir -p $(GOCACHE_DIR)
	@$(GO) run ./cmd/hexlet-go-crawler $(URL)
