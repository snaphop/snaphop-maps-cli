.PHONY: build test coverage lint fmt check dist clean

# Every statement is covered, and it stays that way (AGENTS.md).
COVERAGE_MIN ?= 100.0
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)
BUILD := CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)"

# The platforms `make dist` builds, one job each; `make -j dist` builds them concurrently.
PLATFORMS := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64
BINARIES := $(foreach p,$(PLATFORMS),dist/snaphop-maps-$(subst /,-,$(p))$(if $(findstring windows,$(p)),.exe,))

build:
	$(BUILD) -o bin/snaphop-maps ./cmd/snaphop-maps

test:
	go test -race -shuffle=on ./...

coverage:
	COVERAGE_MIN="$(COVERAGE_MIN)" ./scripts/coverage.sh

lint:
	test -z "$$(gofmt -l .)"
	go vet ./...

fmt:
	gofmt -w .

check: lint build coverage

dist: $(BINARIES)
	cd dist && sha256sum snaphop-maps-* > SHA256SUMS

dist/snaphop-maps-%:
	@mkdir -p dist
	platform=$$(echo "$*" | sed 's/\.exe$$//'); \
	GOOS=$${platform%-*} GOARCH=$${platform#*-} $(BUILD) -o $@ ./cmd/snaphop-maps

clean:
	rm -rf bin dist coverage.out
