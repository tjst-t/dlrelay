FROM golang:1.24-alpine AS builder
RUN apk add --no-cache git
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /bin/dlrelay-server ./cmd/server

FROM alpine:3.21
RUN apk add --no-cache ffmpeg ca-certificates
COPY --from=builder /bin/dlrelay-server /usr/local/bin/
EXPOSE 8090
ENTRYPOINT ["dlrelay-server"]
