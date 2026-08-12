.PHONY: build dev-down dev-up release-check release-snapshot run test test-e2e test-install test-race

GO_ENV := GOCACHE=/tmp/nats-tui-go-cache GOMODCACHE=/tmp/nats-tui-modcache

build:
	$(GO_ENV) go build -o bin/nats-tui ./cmd/nats-tui

run:
	$(GO_ENV) go run ./cmd/nats-tui

test:
	$(GO_ENV) go test ./...

test-race:
	$(GO_ENV) go test -race ./...

test-e2e:
	$(GO_ENV) go test -tags=e2e ./internal/ports/tui -run '^TestE2E'

test-install:
	sh ./scripts/test-install.sh

release-check:
	goreleaser check

release-snapshot:
	goreleaser release --snapshot --clean

dev-up:
	docker compose up -d nats

dev-down:
	docker compose down
