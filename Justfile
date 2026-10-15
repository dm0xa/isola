# Justfile - run with `just <command>`
# Install: brew install just

# Start the single-file stack (recommended for first-time users)
up:
    docker compose up --build -d

# Stop the single-file stack
down:
    docker compose down

# Follow logs of the stack
logs:
    docker compose logs -f

# Start all services in development mode (full stack: nginx + dev frontend + backend)
up-dev:
    docker compose -f deploy/docker-compose.yml up -d

# Stop all services
down-dev:
    docker compose -f deploy/docker-compose.yml down

# View logs
logs-dev:
    docker compose -f deploy/docker-compose.yml logs -f

# Nginx — test config validity
nginx-test:
    docker compose -f deploy/docker-compose.yml exec nginx nginx -t

# Nginx — reload config without downtime
nginx-reload:
    docker compose -f deploy/docker-compose.yml exec nginx nginx -s reload

# Nginx logs
nginx-logs:
    docker compose -f deploy/docker-compose.yml logs -f nginx

# Build the Electron sandbox image (Isola transport).
build-browser-node-electron:
    docker build -t isola-browser:electron -f ./browsers/browser-node-electron/Dockerfile ./browsers

# Alias for release scripts that still call build-all-browsers.
build-all-browsers: build-browser-node-electron

# Build and push the sandbox image for both architectures (needs: docker login ghcr.io).
push-browser-image:
    docker buildx build --platform linux/amd64,linux/arm64 -t ghcr.io/dm0xa/isola-browser:electron --push -f ./browsers/browser-node-electron/Dockerfile ./browsers

# Run database migrations
migrate-up:
    docker compose -f deploy/docker-compose.yml exec -T postgres psql -U isola -d isola -f /docker-entrypoint-initdb.d/000001_init.up.sql
    # Alternative: use golang-migrate CLI

# Start frontend dev server (without Docker)
dev-frontend:
    cd frontend && npm run dev

# Build frontend for production
build-frontend:
    cd frontend && npm run build

# Build backend (with version info)
build-backend:
    cd backend && ./scripts/build.sh isola ./cmd/server

# Run backend tests
test-backend:
    cd backend && go test ./...

# Run the streamer tests (WS e2e tests require ffmpeg and FAIL, not skip, without it)
test-streamer:
    cd browsers/isola-streamer && go test ./...

# Run unit tests
test-unit:
    #!/usr/bin/env bash
    set -euo pipefail
    (cd backend && go test ./...)
    (cd browsers/isola-streamer && go test ./...)

# Lint backend
lint-backend:
    cd backend && go vet ./...

# Lint frontend
lint-frontend:
    cd frontend && npm run lint

# Install frontend dependencies
install-frontend:
    cd frontend && npm install

# Tidy backend module files (vendor/ is local-only; Docker uses go mod download)
tidy-backend:
    cd backend && go mod tidy

# Format Go code
fmt-backend:
    cd backend && go fmt ./...

# Full setup from scratch
setup: install-frontend build-frontend tidy-backend
    @echo "Setup complete. Run 'just up' to start."

# Clean all Docker resources
clean:
    docker compose -f deploy/docker-compose.yml down -v
    docker system prune -f
