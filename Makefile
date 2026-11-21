GO ?= go
NPM ?= npm
WEB_DIR := web
BIN := bin/sinkhole
EMBED_DIR := internal/webui/dist
IMAGE ?= network-wide-dns-sinkhole:local

.PHONY: all check fmt fmt-check lint typecheck test test-race build web-build backend-build run web-install web-dev docker-build docker-up docker-down clean

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

build: web-build backend-build

web-build:
	$(NPM) --prefix $(WEB_DIR) run build

backend-build: web-build
	@mkdir -p bin
	@rm -rf $(EMBED_DIR)
	@mkdir -p $(EMBED_DIR)
	@cp -R $(WEB_DIR)/dist/. $(EMBED_DIR)/
	CGO_ENABLED=0 $(GO) build -tags embedded -trimpath -ldflags="-s -w" -o $(BIN) ./cmd/sinkhole

run:
	$(GO) run ./cmd/sinkhole

web-install:
	$(NPM) --prefix $(WEB_DIR) ci

web-dev:
	$(NPM) --prefix $(WEB_DIR) run dev

docker-build:
	docker build -t $(IMAGE) .

docker-up:
	docker compose up -d --build

docker-down:
	docker compose down

clean:
	rm -rf bin $(WEB_DIR)/dist $(EMBED_DIR)

check: fmt-check lint typecheck test build
