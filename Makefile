.PHONY: help run build tidy test fmt vet lint migrate-up migrate-down migrate-status docker-up docker-down docker-logs deploy

SHELL := /bin/sh
APP_NAME := gpt2api
BIN_DIR := bin

CONFIG ?= configs/config.yaml
DSN ?= $(shell awk '/mysql:/,/^[^ ]/' $(CONFIG) | grep -E "^\s*dsn:" | head -1 | sed 's/.*dsn:\s*//;s/"//g')

help:
	@echo "Targets:"
	@echo "  run              - go run cmd/server"
	@echo "  build            - build binary to bin/$(APP_NAME)"
	@echo "  tidy             - go mod tidy"
	@echo "  test             - go test ./..."
	@echo "  fmt              - gofmt -w"
	@echo "  vet              - go vet ./..."
	@echo "  migrate-up       - goose up"
	@echo "  migrate-down     - goose down"
	@echo "  migrate-status   - goose status"
	@echo "  docker-up        - docker compose up -d"
	@echo "  docker-down      - docker compose down"
	@echo "  docker-logs      - docker compose logs -f"
	@echo "  deploy           - unified initial deploy/update flow"

run:
	go run ./cmd/server

build:
	@mkdir -p $(BIN_DIR)
	go build -ldflags "-s -w" -o $(BIN_DIR)/$(APP_NAME) ./cmd/server

tidy:
	go mod tidy

test:
	go test ./...

fmt:
	gofmt -w .

vet:
	go vet ./...

migrate-up:
	goose -dir sql/migrations mysql "$(DSN)" up

migrate-down:
	goose -dir sql/migrations mysql "$(DSN)" down

migrate-status:
	goose -dir sql/migrations mysql "$(DSN)" status

docker-up:
	docker compose -f deploy/docker-compose.yml up -d

docker-down:
	docker compose -f deploy/docker-compose.yml down

docker-logs:
	docker compose -f deploy/docker-compose.yml logs -f

deploy:
	bash deploy/deploy.sh
