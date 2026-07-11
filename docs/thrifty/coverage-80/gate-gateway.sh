#!/bin/sh
# ABOUTME: Session gate for the thrifty coverage-80 run, gateway package.
# ABOUTME: Checks coverage >=80%, package lint, and race detector; exits nonzero on any failure.
set -e
cd "$(git rev-parse --show-toplevel)"
go test -count=1 -coverprofile=/tmp/gate-gateway.out ./internal/gateway/
pct=$(go tool cover -func=/tmp/gate-gateway.out | awk '/^total:/ {gsub(/%/,"",$NF); print $NF}')
echo "gateway coverage: ${pct}%"
awk -v p="$pct" 'BEGIN { if (p+0 < 80) { print "GATE FAIL: gateway coverage below 80%"; exit 1 } }'
golangci-lint run ./internal/gateway/
go test -race -count=1 ./internal/gateway/
echo "GATE PASS (gateway)"
