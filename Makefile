.PHONY: build test coverage lint fmt check security dist clean FORCE

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

security:
	python3 -m unittest discover -s security -p 'test_*.py' -v
	python3 security/check.py

dist: $(BINARIES) dist/snaphop-maps-skill.zip
	cd dist && sha256sum snaphop-maps-* > SHA256SUMS

# Every file in dist is rebuilt each time, from the source and VERSION as they are now: Go's build cache
# makes that cheap, and a file kept from an earlier build would go out under a new SHA256SUMS.
FORCE:

# The Agent Skill as a zip to upload, packed by the CLI itself (ADR 0003).
dist/snaphop-maps-skill.zip: FORCE
	@mkdir -p dist
	go run ./cmd/snaphop-maps skill pack --output $@

dist/snaphop-maps-%: FORCE
	@mkdir -p dist
	platform=$$(echo "$*" | sed 's/\.exe$$//'); \
	GOOS=$${platform%-*} GOARCH=$${platform#*-} $(BUILD) -o $@ ./cmd/snaphop-maps

clean:
	rm -rf bin dist coverage.out
