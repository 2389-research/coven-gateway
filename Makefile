# ABOUTME: Build and development commands for coven-gateway
# ABOUTME: Handles proto generation, building, and testing

.PHONY: all build build-gateway build-admin proto update-proto clean test lint lint-go lint-md fmt run setup hooks web web-deps web-tokens web-dev web-clean

# Default target
all: proto web build

# Build all binaries
# Note: coven-tui is in the Rust coven repo: https://github.com/2389-research/coven
build: build-gateway build-admin

build-gateway:
	go build -o bin/coven-gateway ./cmd/coven-gateway

build-admin:
	go build -o bin/coven-admin ./cmd/coven-admin

# Generate protobuf code from shared proto submodule
proto:
	@mkdir -p proto/coven
	protoc \
		--go_out=. \
		--go_opt=module=github.com/2389/coven-gateway \
		--go_opt=Mcoven.proto=github.com/2389/coven-gateway/proto/coven \
		--go-grpc_out=. \
		--go-grpc_opt=module=github.com/2389/coven-gateway \
		--go-grpc_opt=Mcoven.proto=github.com/2389/coven-gateway/proto/coven \
		-I proto/coven-proto \
		proto/coven-proto/coven.proto

# Update proto submodule and regenerate
update-proto:
	git submodule update --remote proto/coven-proto
	$(MAKE) proto
	@echo "Proto updated from coven-proto submodule"

# Install protoc plugins (run once)
proto-deps:
	go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

# Run the server
run: build
	COVEN_CONFIG=config.yaml ./bin/coven-gateway serve

# Run tests
test:
	go test -v ./...

# Run linters
lint: lint-go lint-md

# Run Go linter
lint-go:
	golangci-lint run

# Run Markdown linter
lint-md:
	@command -v markdownlint >/dev/null 2>&1 || { echo "markdownlint not found. Install: npm install -g markdownlint-cli"; exit 1; }
	markdownlint --config .markdownlint.yaml '**/*.md' --ignore 'docs/archive/' --ignore 'docs/plans/' --ignore 'node_modules/' --ignore 'proto/'

# Format code
fmt:
	go fmt ./...

# Frontend: install dependencies (idempotent via lockfile check)
web-deps:
	cd web && npm ci --silent

# Frontend: generate CSS from design tokens
web-tokens: web-deps
	cd web && npx tsx scripts/build-tokens.ts

# Frontend: production build (tokens → Vite → copy to embed dir)
web: web-tokens
	cd web && npm run build
	rm -rf internal/assets/dist
	cp -r web/dist internal/assets/dist
	touch internal/assets/dist/.gitkeep

# Frontend: Vite dev server with HMR
web-dev:
	cd web && npm run dev

# Frontend: clean built assets
web-clean:
	rm -rf web/dist
	rm -rf internal/assets/dist
	mkdir -p internal/assets/dist
	touch internal/assets/dist/.gitkeep

# Clean build artifacts
clean: web-clean
	rm -rf bin/
	rm -rf proto/coven/*.pb.go

# Development: watch and rebuild
dev:
	@echo "Run: go run ./cmd/coven-gateway serve --config config.yaml"

# Install git hooks (symlink so updates are automatic)
hooks:
	@echo "Installing pre-commit hook..."
	@ln -sf ../../scripts/pre-commit .git/hooks/pre-commit
	@echo "Pre-commit hook installed!"

# Setup development environment (run after cloning)
setup: proto-deps hooks
	@git submodule update --init --recursive
	@echo "Development environment ready!"
