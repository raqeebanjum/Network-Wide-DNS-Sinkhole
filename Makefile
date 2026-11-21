GO ?= go
NPM ?= npm
WEB_DIR := web
BIN := bin/sinkhole

.PHONY: all check fmt fmt-check lint typecheck test test-race build run web-install web-dev

all: check

fmt:
	$(GO) fmt ./...
	$(NPM) --prefix $(WEB_DIR) run format

fmt-check:
	@files="$$(gofmt -l cmd internal)"; \
	test -z "$$files" || { echo "Go files need formatting:"; echo "$$files"; exit 1; }
	$(NPM) --prefix $(WEB_DIR) run format:check

lint:
	$(GO) vet ./...
	$(NPM) --prefix $(WEB_DIR) run lint

typecheck:
	$(NPM) --prefix $(WEB_DIR) run typecheck

test:
	$(GO) test ./...
	$(NPM) --prefix $(WEB_DIR) run test

test-race:
	$(GO) test -race ./...

build:
	@mkdir -p bin
	$(GO) build -trimpath -o $(BIN) ./cmd/sinkhole
	$(NPM) --prefix $(WEB_DIR) run build

run:
	$(GO) run ./cmd/sinkhole

web-install:
	$(NPM) --prefix $(WEB_DIR) ci

web-dev:
	$(NPM) --prefix $(WEB_DIR) run dev

check: fmt-check lint typecheck test build
