FROM golang:1.25-alpine AS builder
ARG VERSION=dev
RUN apk add --no-cache git
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w -X github.com/tjst-t/dlrelay/internal/version.Version=${VERSION}" -o /bin/dlrelay-server ./cmd/server

FROM alpine:3.21
RUN apk add --no-cache ffmpeg ca-certificates nodejs python3 py3-pip \
    && pip3 install --no-cache-dir --break-system-packages yt-dlp curl_cffi
COPY --from=builder /bin/dlrelay-server /usr/local/bin/
EXPOSE 8090
ENTRYPOINT ["dlrelay-server"]
