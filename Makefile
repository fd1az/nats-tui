.PHONY: build dev-down dev-up run test test-race

GO_ENV := GOCACHE=/tmp/nats-tui-go-cache GOMODCACHE=/tmp/nats-tui-modcache

build:
	$(GO_ENV) go build -o bin/nats-tui ./cmd/nats-tui

run:
	$(GO_ENV) go run ./cmd/nats-tui

test:
	$(GO_ENV) go test ./...

test-race:
	$(GO_ENV) go test -race ./...

dev-up:
	docker compose up -d nats

dev-down:
	docker compose down
