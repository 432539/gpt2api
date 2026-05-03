# KleinAI Backend Makefile
# 端口约定：api=17180  admin=17188  openai=17200  pprof=17590

ENV ?= dev
APP_PKG := github.com/kleinai/backend
LDFLAGS := -s -w -X $(APP_PKG)/pkg/version.Build=$(shell git rev-parse --short HEAD 2>/dev/null || echo dev) \
                 -X $(APP_PKG)/pkg/version.Time=$(shell date -u +%Y-%m-%dT%H:%M:%SZ)

GO ?= go
GOFLAGS ?= -trimpath
BIN_DIR := bin

.PHONY: help tidy lint vet test build run-api run-admin run-openai run-worker \
        migrate-up migrate-down docker fmt clean

help:
	@echo "KleinAI backend make targets:"
	@echo "  tidy         go mod tidy"
	@echo "  fmt          gofmt + goimports"
	@echo "  lint         golangci-lint"
	@echo "  vet          go vet"
	@echo "  test         go test ./... -race"
	@echo "  build        编译全部 4 个二进制到 bin/"
	@echo "  run-api      启动用户端 API（:17180）"
	@echo "  run-admin    启动管理后台 API（:17188）"
	@echo "  run-openai   启动 OpenAI 兼容 API（:17200）"
	@echo "  run-worker   启动异步任务消费者"
	@echo "  migrate-up   数据库向上迁移"
	@echo "  migrate-down 回滚一个版本"

tidy:
	$(GO) mod tidy

fmt:
	$(GO) fmt ./...
	@command -v goimports >/dev/null && goimports -w . || echo "goimports not installed"

lint:
	@command -v golangci-lint >/dev/null && golangci-lint run ./... || echo "golangci-lint not installed; run: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"

vet:
	$(GO) vet ./...

test:
	$(GO) test ./... -race -count=1

build:
	@mkdir -p $(BIN_DIR)
	$(GO) build $(GOFLAGS) -ldflags="$(LDFLAGS)" -o $(BIN_DIR)/api    ./cmd/api
	$(GO) build $(GOFLAGS) -ldflags="$(LDFLAGS)" -o $(BIN_DIR)/admin  ./cmd/admin
	$(GO) build $(GOFLAGS) -ldflags="$(LDFLAGS)" -o $(BIN_DIR)/openai ./cmd/openai
	$(GO) build $(GOFLAGS) -ldflags="$(LDFLAGS)" -o $(BIN_DIR)/worker ./cmd/worker

run-api:
	KLEIN_ENV=$(ENV) $(GO) run ./cmd/api

run-admin:
	KLEIN_ENV=$(ENV) $(GO) run ./cmd/admin

run-openai:
	KLEIN_ENV=$(ENV) $(GO) run ./cmd/openai

run-worker:
	KLEIN_ENV=$(ENV) $(GO) run ./cmd/worker

migrate-up:
	@command -v goose >/dev/null || { echo "install: go install github.com/pressly/goose/v3/cmd/goose@latest"; exit 1; }
	goose -dir migrations mysql "$(KLEIN_DB_DSN)" up

migrate-down:
	goose -dir migrations mysql "$(KLEIN_DB_DSN)" down

clean:
	rm -rf $(BIN_DIR)
