#!/usr/bin/env bash
# ABOUTME: Verifies the production Docker image embeds its Vite frontend assets.
# ABOUTME: Starts the real gateway image and checks that its login module is reachable.

set -euo pipefail

usage() {
    cat <<'EOF'
Usage: scripts/check-docker-assets.sh [--help]

Build the coven-gateway Docker image, start it on an automatically assigned
localhost port, and verify that the login page uses a reachable embedded asset.

Environment:
  COVEN_DOCKER_TEST_IMAGE  Image tag to build and test
                           (default: coven-gateway:frontend-assets-test)
EOF
}

if [[ $# -gt 1 ]]; then
    usage >&2
    echo "Error: unexpected arguments: $*" >&2
    exit 2
fi

if [[ $# -eq 1 ]]; then
    case "$1" in
        --help)
            usage
            exit 0
            ;;
        *)
            usage >&2
            echo "Error: unexpected argument: $1" >&2
            exit 2
            ;;
    esac
fi

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(dirname "$SCRIPT_DIR")"
IMAGE_TAG="${COVEN_DOCKER_TEST_IMAGE:-coven-gateway:frontend-assets-test}"
DIAGNOSTIC_DIR="$(mktemp -d "${TMPDIR:-/tmp}/coven-docker-assets.XXXXXX")"
CONTAINER_ID=""
CURL_OPTIONS=(--connect-timeout 2 --max-time 5 --silent --show-error)

cleanup() {
    local status=$?

    trap - EXIT
    if [[ -n "$CONTAINER_ID" ]]; then
        if ! docker rm --force "$CONTAINER_ID" >"$DIAGNOSTIC_DIR/container-remove.log" 2>&1; then
            echo "Failed to remove test container: $CONTAINER_ID" >&2
            tail -n 40 "$DIAGNOSTIC_DIR/container-remove.log" >&2
            if [[ $status -eq 0 ]]; then
                status=1
            fi
        fi
    fi

    if [[ $status -eq 0 ]]; then
        rm -rf "$DIAGNOSTIC_DIR"
    else
        echo "Diagnostics preserved at: $DIAGNOSTIC_DIR" >&2
    fi

    exit "$status"
}
trap cleanup EXIT

require_command() {
    local command_name=$1

    if ! command -v "$command_name" >/dev/null 2>&1; then
        echo "Error: $command_name is required" >&2
        exit 1
    fi
}

capture_container_logs() {
    if [[ -z "$CONTAINER_ID" ]]; then
        return
    fi

    docker logs "$CONTAINER_ID" >"$DIAGNOSTIC_DIR/container.log" 2>&1 || true
    echo "Last 40 lines of container logs:" >&2
    tail -n 40 "$DIAGNOSTIC_DIR/container.log" >&2
}

require_command docker
require_command curl

cd "$ROOT_DIR"

BUILD_LOG="$DIAGNOSTIC_DIR/build.log"
if docker buildx version >/dev/null 2>&1 && docker buildx inspect >/dev/null 2>&1; then
    BUILD_MODE="Buildx"
    BUILD_COMMAND=(docker buildx build --load --tag "$IMAGE_TAG" .)
else
    BUILD_MODE="legacy"
    BUILD_COMMAND=(docker build --tag "$IMAGE_TAG" .)
fi

echo "Docker test image: $IMAGE_TAG"
echo "Docker build mode: $BUILD_MODE"
echo "Docker build log: $BUILD_LOG"

if ! "${BUILD_COMMAND[@]}" >"$BUILD_LOG" 2>&1; then
    echo "Docker image build failed. Last 40 lines:" >&2
    tail -n 40 "$BUILD_LOG" >&2
    exit 1
fi

RUN_LOG="$DIAGNOSTIC_DIR/docker-run.log"
if ! CONTAINER_ID="$(docker run --detach \
    --publish 127.0.0.1::8080 \
    --env COVEN_JWT_SECRET='docker-assets-test-secret-at-least-32-bytes' \
    --volume "$ROOT_DIR/config.example.yaml:/app/config.yaml:ro" \
    "$IMAGE_TAG" 2>"$RUN_LOG")"; then
    echo "Docker container failed to start:" >&2
    tail -n 40 "$RUN_LOG" >&2
    exit 1
fi

if ! PORT_OUTPUT="$(docker port "$CONTAINER_ID" 8080/tcp 2>"$DIAGNOSTIC_DIR/docker-port.log")"; then
    echo "Could not determine the gateway's published port" >&2
    echo "Captured docker port output: ${PORT_OUTPUT:-<empty>}" >&2
    tail -n 40 "$DIAGNOSTIC_DIR/docker-port.log" >&2
    capture_container_logs
    exit 1
fi

if [[ "$PORT_OUTPUT" =~ ^127\.0\.0\.1:([0-9]+)$ ]]; then
    PORT="${BASH_REMATCH[1]}"
else
    echo "Docker published port output is not exactly one localhost port" >&2
    echo "Captured docker port output: ${PORT_OUTPUT:-<empty>}" >&2
    tail -n 40 "$DIAGNOSTIC_DIR/docker-port.log" >&2
    capture_container_logs
    exit 1
fi

if ((10#$PORT < 1 || 10#$PORT > 65535)); then
    echo "Docker published port is outside the valid range: $PORT" >&2
    echo "Captured docker port output: $PORT_OUTPUT" >&2
    tail -n 40 "$DIAGNOSTIC_DIR/docker-port.log" >&2
    capture_container_logs
    exit 1
fi

BASE_URL="http://127.0.0.1:$PORT"

HEALTHY=false
for _ in {1..60}; do
    HEALTH_STATUS=""
    if HEALTH_STATUS="$(curl "${CURL_OPTIONS[@]}" \
        --output "$DIAGNOSTIC_DIR/health-response" \
        --write-out '%{http_code}' \
        "$BASE_URL/health" 2>"$DIAGNOSTIC_DIR/health-curl.log")"; then
        printf '%s\n' "$HEALTH_STATUS" >"$DIAGNOSTIC_DIR/health-status"
        if [[ "$HEALTH_STATUS" == "200" ]]; then
            HEALTHY=true
            break
        fi
    fi
    sleep 0.5
done

if [[ "$HEALTHY" != true ]]; then
    echo "Gateway did not become healthy at $BASE_URL/health" >&2
    capture_container_logs
    exit 1
fi

LOGIN_HTML="$DIAGNOSTIC_DIR/login.html"
if ! LOGIN_STATUS="$(curl "${CURL_OPTIONS[@]}" \
    --location \
    --output "$LOGIN_HTML" \
    --write-out '%{http_code}' \
    "$BASE_URL/login" 2>"$DIAGNOSTIC_DIR/login-curl.log")"; then
    echo "Failed to fetch the login page" >&2
    tail -n 40 "$DIAGNOSTIC_DIR/login-curl.log" >&2
    capture_container_logs
    exit 1
fi
printf '%s\n' "$LOGIN_STATUS" >"$DIAGNOSTIC_DIR/login-status"

if [[ "$LOGIN_STATUS" != "200" ]]; then
    echo "Login page returned final HTTP status $LOGIN_STATUS, expected 200" >&2
    tail -n 40 "$DIAGNOSTIC_DIR/login-curl.log" >&2
    capture_container_logs
    exit 1
fi

if grep --quiet --fixed-strings 'localhost:5173' "$LOGIN_HTML"; then
    echo "Login page references the Vite development server" >&2
    exit 1
fi

ASSET_PATH="$(sed -n '/<script type="module" src="/{s/.*<script type="module" src="\([^"]*\)".*/\1/p;q;}' "$LOGIN_HTML")"
if [[ -z "$ASSET_PATH" ]]; then
    echo "Login page does not contain a module script" >&2
    exit 1
fi

case "$ASSET_PATH" in
    /static/*) ;;
    *)
        echo "Login page module script is not an embedded /static/ asset: $ASSET_PATH" >&2
        exit 1
        ;;
esac

ASSET_RESPONSE="$DIAGNOSTIC_DIR/asset-response"
ASSET_METADATA="$DIAGNOSTIC_DIR/asset-metadata"
if ! curl "${CURL_OPTIONS[@]}" \
    --output "$ASSET_RESPONSE" \
    --write-out '%{http_code}\n%{content_type}\n' \
    "$BASE_URL$ASSET_PATH" >"$ASSET_METADATA" 2>"$DIAGNOSTIC_DIR/asset-curl.log"; then
    echo "Failed to fetch embedded frontend asset: $ASSET_PATH" >&2
    tail -n 40 "$DIAGNOSTIC_DIR/asset-curl.log" >&2
    exit 1
fi

ASSET_STATUS="$(sed -n '1p' "$ASSET_METADATA")"
ASSET_CONTENT_TYPE="$(sed -n '2p' "$ASSET_METADATA")"
ASSET_MEDIA_TYPE="${ASSET_CONTENT_TYPE%%;*}"

if [[ "$ASSET_STATUS" != "200" ]]; then
    echo "Embedded frontend asset returned HTTP status $ASSET_STATUS, expected 200" >&2
    exit 1
fi

if [[ ! -s "$ASSET_RESPONSE" ]]; then
    echo "Embedded frontend asset response is empty: $ASSET_PATH" >&2
    exit 1
fi

if [[ "$ASSET_MEDIA_TYPE" != "application/javascript" ]]; then
    echo "Embedded frontend asset has unexpected content type: ${ASSET_CONTENT_TYPE:-<empty>}" >&2
    exit 1
fi

echo "Docker frontend assets are embedded and reachable"
