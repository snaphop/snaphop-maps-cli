#!/bin/sh
# Runs every test with the race detector, in random order, instrumenting every package in the
# module, and fails when statement coverage is under COVERAGE_MIN (100.0 unless told otherwise).
# Coverage is counted from the profile's statements, not from the rounded percentage go tool cover
# prints, which calls 99.95% 100.0%.
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

# Each package's test binary lists every block in the module; a block is covered when any of them ran it.
counts=$(awk 'NR > 1 {
  statements[$1] = $2
  if ($3 > 0) covered[$1] = 1
}
END {
  for (block in statements) {
    total += statements[block]
    if (block in covered) hit += statements[block]
    else print "not covered: " block > "/dev/stderr"
  }
  printf "%d %d\n", hit, total
}' coverage.out)
hit=${counts% *}
total=${counts#* }

if ! awk -v hit="$hit" -v total="$total" -v minimum="$coverage_min" \
  'BEGIN { exit !(total > 0 && hit * 100 >= minimum * total) }'; then
  echo "coverage ${hit} of ${total} statements is below the ${coverage_min}% floor" >&2
  exit 1
fi
printf 'coverage %d of %d statements meets the %s%% floor\n' "$hit" "$total" "$coverage_min"
