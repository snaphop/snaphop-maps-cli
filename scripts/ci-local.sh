#!/bin/sh
# The handoff check: what Verify runs, on this machine.
set -eu
cd "$(dirname "$0")/.."
make check
make -j dist
go run golang.org/x/vuln/cmd/govulncheck@v1.8.0 ./...
git diff --check
