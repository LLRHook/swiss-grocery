.PHONY: up down api api-build optimizer-build api-test api-fmt-check scraper migrate mobile

GO ?= go
CARGO ?= cargo
RUST_TOOLCHAIN ?= 1.93.1
API_BUILD_DIR := $(CURDIR)/.build/api
OPTIMIZER_MANIFEST := services/api/optimizer/Cargo.toml
CARGO_TARGET_DIR ?= $(CURDIR)/.build/cargo
export CARGO_TARGET_DIR
export GOTOOLCHAIN := local

# Start all services
up:
	docker compose up -d

# Stop all services
down:
	docker compose down

# Keep the required native executable beside the API for both local and CI builds.
optimizer-build:
	$(CARGO) +$(RUST_TOOLCHAIN) build --manifest-path $(OPTIMIZER_MANIFEST) --release --locked --offline
	mkdir -p "$(API_BUILD_DIR)"
	cp "$(CARGO_TARGET_DIR)/release/swiss-optimizer" "$(API_BUILD_DIR)/swiss-optimizer"

api-build: optimizer-build
	cd services/api && CGO_ENABLED=0 $(GO) build -o "$(API_BUILD_DIR)/server" .

# Run API locally (requires Go >=1.26.1 and the pinned Rust toolchain).
api: api-build
	"$(API_BUILD_DIR)/server"

# No database, retailer, mobile runtime or Docker is used by these tests.
# Compile host test executables beside the very same native release artifact,
# then run them there to exercise the default executable-relative path.
api-test: api-build
	SWISS_OPTIMIZER_TEST_BIN="$(API_BUILD_DIR)/swiss-optimizer" $(CARGO) +$(RUST_TOOLCHAIN) test --manifest-path $(OPTIMIZER_MANIFEST) --locked --offline
	cd services/api && $(GO) test -c -o "$(API_BUILD_DIR)/" ./...
	SWISS_OPTIMIZER_BIN= $(GO) tool test2json -p github.com/LLRHook/swiss-grocery/services/api/internal/optimizer -t "$(API_BUILD_DIR)/optimizer.test" -test.v=test2json -test.timeout=120s
	SWISS_OPTIMIZER_BIN= $(GO) tool test2json -p github.com/LLRHook/swiss-grocery/services/api/internal/handlers -t "$(API_BUILD_DIR)/handlers.test" -test.v=test2json -test.timeout=120s

api-fmt-check:
	test -z "$$(gofmt -l services/api/internal/optimizer/*.go services/api/internal/handlers/lists*.go)"
	$(CARGO) +$(RUST_TOOLCHAIN) fmt --manifest-path $(OPTIMIZER_MANIFEST) -- --check

# Run scraper locally (requires Python)
scraper:
	cd services/scraper && python -m src.main

# Run mobile app (requires Node/Expo)
mobile:
	cd apps/mobile && npx expo start

# Run DB migrations (via psql)
migrate:
	docker compose exec postgres psql -U postgres -d swissgrocery -f /docker-entrypoint-initdb.d/001_initial.sql

# Fresh start — wipe DB and rebuild
reset:
	docker compose down -v
	docker compose up -d
