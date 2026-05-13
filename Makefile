.PHONY: build test cover lint tidy record-cassettes

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

# 重录 LLM cassette。用真实 API key 跑一次，把 fixture 落到 testdata/cassettes/。
# CI 跑回归一律走 ModeReplayOnly，永远不会触发录制。
#
# 用法（需要 ANTHROPIC_API_KEY 或 OPENROUTER_API_KEY）：
#   make record-cassettes              # 只重录 internal/agent 下的回放测试
record-cassettes:
	WHISPERER_VCR_RECORD=1 $(GO) test -count=1 -tags=cassette ./internal/agent/...
