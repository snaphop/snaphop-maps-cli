#!/bin/sh
# Runs every test with the race detector, in random order, instrumenting every package in the
# module, and fails when statement coverage is under COVERAGE_MIN (100.0 unless told otherwise).
set -eu
cd "$(dirname "$0")/.."

coverage_min=${COVERAGE_MIN:-100.0}

if ! awk -v minimum="$coverage_min" 'BEGIN {
  exit !(minimum ~ /^[0-9]+([.][0-9]+)?$/ && minimum >= 0 && minimum <= 100)
}'; then
  echo "COVERAGE_MIN must be a percentage from 0 to 100" >&2
  exit 2
fi

go test -race -shuffle=on -count=1 -covermode=atomic -coverpkg=./... \
  -coverprofile=coverage.out ./...

coverage_report=$(go tool cover -func=coverage.out)
printf '%s\n' "$coverage_report" | grep -v '100.0%$' || true
coverage_total=$(printf '%s\n' "$coverage_report" | awk '/^total:/ {
  gsub(/%/, "", $NF)
  print $NF
}')

if ! awk -v total="$coverage_total" -v minimum="$coverage_min" \
  'BEGIN { exit !(total >= minimum) }'; then
  echo "coverage ${coverage_total}% is below the ${coverage_min}% floor" >&2
  exit 1
fi
printf 'coverage %.1f%% meets the %.1f%% floor\n' "$coverage_total" "$coverage_min"
