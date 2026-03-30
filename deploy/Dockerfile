FROM golang:1.24-bookworm AS builder

WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/douyin-recorder ./cmd/app

FROM debian:bookworm-slim AS runtime

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates ffmpeg python3 python3-pip sqlite3 \
    && pip3 install --no-cache-dir --break-system-packages streamlink \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app
COPY --from=builder /out/douyin-recorder /app/douyin-recorder

ENV APP_LISTEN_ADDR=:8080 \
    APP_DB_PATH=/data/db/recorder.db \
    APP_RECORDINGS_ROOT=/data/recordings \
    APP_COOKIES_ROOT=/data/cookies \
    APP_ADMIN_USERNAME=admin \
    APP_ADMIN_PASSWORD=admin123456

EXPOSE 8080
ENTRYPOINT ["/app/douyin-recorder"]
