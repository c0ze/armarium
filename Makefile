# Armarium build. `make` builds the UI and a static binary into bin/.
GO      ?= go
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)

.PHONY: all build web test check docker clean

all: web build

build:
	CGO_ENABLED=0 $(GO) build -trimpath -ldflags '$(LDFLAGS)' -o bin/armarium ./cmd/armarium

web:
	cd web && npm ci --no-audit --no-fund && npm run build

test:
	$(GO) test ./...
	cd web && npm test

# The CI gate: vet, staticcheck (if installed), Go tests, svelte-check, UI tests.
check:
	$(GO) vet ./...
	@if command -v staticcheck >/dev/null; then staticcheck ./...; else echo "staticcheck not installed, skipped"; fi
	$(GO) test ./...
	cd web && npm run check && npm test

docker:
	docker build --build-arg VERSION=$(VERSION) -t armarium:$(VERSION) -f deploy/Dockerfile .

clean:
	rm -rf bin
	find web/dist -mindepth 1 ! -name .keep -delete
