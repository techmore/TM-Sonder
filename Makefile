# TM Sonder Go server
# Targets: bare-metal macOS (launchd) and Apple Containerization micro-VM.

GO      ?= go
BINARY  := sonder
VERSION ?= $(shell date -u +%Y%m%d.%H%M%S)
LDFLAGS := -s -w -X main.version=$(VERSION)

.PHONY: build mac linux test vet fmt clean install-launchd uninstall-launchd container-build container-run

build: mac

status-app: ## Build the lightweight macOS menu-bar service indicator
	mkdir -p "bin/Sonder Status.app/Contents/MacOS"
	xcrun swiftc -swift-version 6 -O -framework AppKit tools/SonderStatus/main.swift -o "bin/Sonder Status.app/Contents/MacOS/SonderStatus"
	cp tools/SonderStatus/Info.plist "bin/Sonder Status.app/Contents/Info.plist"

mac: ## darwin/arm64 optimized binary in bin/
	cd server && CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o ../bin/$(BINARY)-darwin-arm64 ./cmd/sonder

linux: ## linux/arm64 static binary for containers
	cd server && CGO_ENABLED=0 GOOS=linux GOARCH=arm64 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o ../bin/$(BINARY)-linux-arm64 ./cmd/sonder

test:
	node --test server/internal/httpapi/web/library.test.cjs
	cd server && $(GO) test -race -count=1 ./...

vet:
	cd server && $(GO) vet ./...

fmt:
	cd server && $(GO) fmt ./...

clean:
	rm -rf bin

install-launchd: ## build + install + bootstrap + health check
	./deploy/install-launchd.sh

uninstall-launchd:
	./deploy/uninstall-launchd.sh

container-build: ## Apple `container` CLI micro-VM image
	container build -t tm-sonder -f deploy/Containerfile .

container-run: ## run with ./media read-only + persistent sonder-data volume
	container run --name tm-sonder --rm \
	  -p 8797:8797 \
	  --volume "$(PWD)/media:/media:ro" \
	  --volume sonder-data:/data \
	  tm-sonder
