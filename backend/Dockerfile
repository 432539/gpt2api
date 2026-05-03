# syntax=docker/dockerfile:1.7
# ----- 多阶段构建：构建 4 个独立二进制并打入同一镜像 -----
FROM golang:1.22-alpine AS builder

ENV CGO_ENABLED=0 GOOS=linux GOARCH=amd64

WORKDIR /src

RUN apk add --no-cache git ca-certificates tzdata

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG VERSION=dev
ARG BUILD_TIME=unknown

RUN go build -trimpath -ldflags="-s -w \
        -X github.com/kleinai/backend/pkg/version.Build=${VERSION} \
        -X github.com/kleinai/backend/pkg/version.Time=${BUILD_TIME}" \
    -o /out/api    ./cmd/api  && \
    go build -trimpath -ldflags="-s -w \
        -X github.com/kleinai/backend/pkg/version.Build=${VERSION}" \
    -o /out/admin  ./cmd/admin && \
    go build -trimpath -ldflags="-s -w \
        -X github.com/kleinai/backend/pkg/version.Build=${VERSION}" \
    -o /out/openai ./cmd/openai && \
    go build -trimpath -ldflags="-s -w \
        -X github.com/kleinai/backend/pkg/version.Build=${VERSION}" \
    -o /out/worker ./cmd/worker

# ----- 运行阶段：distroless · 非 root -----
FROM gcr.io/distroless/static-debian12:nonroot

WORKDIR /app

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /usr/share/zoneinfo                 /usr/share/zoneinfo
COPY --from=builder /out/api                            /app/api
COPY --from=builder /out/admin                          /app/admin
COPY --from=builder /out/openai                         /app/openai
COPY --from=builder /out/worker                         /app/worker
COPY --from=builder /src/configs                        /app/configs

ENV TZ=Asia/Shanghai
USER 65532:65532

EXPOSE 17180 17188 17200

ENTRYPOINT ["/app/api"]
