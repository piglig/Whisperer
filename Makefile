.PHONY: build test cover lint tidy

GO ?= go
COVER_THRESHOLD ?= 85

build:
	$(GO) build ./...

test:
	$(GO) test -race ./...

cover:
	$(GO) test -race -covermode=atomic -coverprofile=cover.out ./internal/...
	@$(GO) tool cover -func=cover.out | tail -1
	@total=$$($(GO) tool cover -func=cover.out | awk '/^total:/ {print $$3}' | tr -d '%'); \
	awk -v t="$$total" -v th="$(COVER_THRESHOLD)" 'BEGIN { if (t+0 < th+0) { printf "coverage %s%% < threshold %s%%\n", t, th; exit 1 } else { printf "coverage %s%% OK (>= %s%%)\n", t, th } }'

lint:
	$(GO) vet ./...
	@command -v golangci-lint >/dev/null 2>&1 && golangci-lint run || echo "golangci-lint not installed, skipping"

tidy:
	$(GO) mod tidy
