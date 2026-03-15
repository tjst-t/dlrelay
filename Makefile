.PHONY: build-server build-all docker-build test test-youtube clean serve pack-extension

export PATH := /usr/local/go/bin:$(PATH)

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
PID_FILE := /tmp/dlrelay-server-dev.pid
LOG_FILE := /tmp/dlrelay-server-dev.log
PORTMAN_ENV := /tmp/dlrelay-server-portman.env

build-server:
	go build -ldflags="-s -w" -o bin/dlrelay-server ./cmd/server

serve: build-server
	@if [ -f $(PID_FILE) ]; then \
	  OLD_PID=$$(cat $(PID_FILE)); \
	  if kill -0 $$OLD_PID 2>/dev/null; then \
	    echo "==> Killing previous process (PID: $$OLD_PID)..."; \
	    kill $$OLD_PID; \
	    for i in $$(seq 1 50); do kill -0 $$OLD_PID 2>/dev/null || break; sleep 0.1; done; \
	    kill -0 $$OLD_PID 2>/dev/null && kill -9 $$OLD_PID 2>/dev/null || true; \
	  fi; \
	  rm -f $(PID_FILE); \
	fi
	@portman env --name api --expose --output $(PORTMAN_ENV)
	@. $(PORTMAN_ENV) && \
	  echo "==> Starting server on port $$API_PORT (log: $(LOG_FILE))" && \
	  LISTEN_ADDR=:$$API_PORT DOWNLOAD_DIR=$${DOWNLOAD_DIR:-/tmp/dlrelay-downloads} EXTENSION_DIR=$(CURDIR)/extension \
	  nohup ./bin/dlrelay-server > $(LOG_FILE) 2>&1 & \
	  echo $$! > $(PID_FILE) && \
	  echo "    PID: $$(cat $(PID_FILE))"

build-all: build-server

docker-build:
	docker build -t ghcr.io/tjst-t/dlrelay-server:$(VERSION) .

test:
	go test ./...

test-browser:
	npx playwright test test/browser/extension-popup.spec.js test/browser/parsing.spec.js

test-youtube:
	xvfb-run npx playwright test test/browser/youtube-real.spec.js

clean:
	rm -rf bin/

pack-extension:
	cd extension && zip -r ../bin/dlrelay-extension.zip . -x "*.svg" ".*"
