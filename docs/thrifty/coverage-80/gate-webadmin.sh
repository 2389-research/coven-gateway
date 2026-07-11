#!/bin/sh
# ABOUTME: Session gate for the thrifty coverage-80 run, webadmin package.
# ABOUTME: Checks coverage >=80%, package lint, and race detector; exits nonzero on any failure.
set -e
cd "$(git rev-parse --show-toplevel)"
go test -count=1 -coverprofile=/tmp/gate-webadmin.out ./internal/webadmin/
pct=$(go tool cover -func=/tmp/gate-webadmin.out | awk '/^total:/ {gsub(/%/,"",$NF); print $NF}')
echo "webadmin coverage: ${pct}%"
awk -v p="$pct" 'BEGIN { if (p+0 < 80) { print "GATE FAIL: webadmin coverage below 80%"; exit 1 } }'
golangci-lint run ./internal/webadmin/
go test -race -count=1 ./internal/webadmin/
echo "GATE PASS (webadmin)"
