.PHONY: migrate-up migrate-down migrate-create migrate-status migrate-force migrate-version help

# Database path. The migration CLI reads SQLITE_DB_PATH (the same variable the
# server reads), so every target below EXPORTS this rather than just echoing
# it. Previously the CLI hardcoded its own path and ignored DB_PATH entirely,
# so these targets reported one file and migrated another.
# Snapshot the inbound value first: DB_PATH defaults from it, and SQLITE_DB_PATH
# is then re-exported from DB_PATH, so reading it directly in both places would
# be a self-reference.
INBOUND_DB_PATH := $(SQLITE_DB_PATH)
DB_PATH ?= $(if $(INBOUND_DB_PATH),$(INBOUND_DB_PATH),mycorrhizal.db)
export SQLITE_DB_PATH := $(DB_PATH)
MIGRATIONS_DIR = ./database/migrations

help: ## Show this help message
	@echo 'Usage: make [target]'
	@echo ''
	@echo 'Available targets:'
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-20s %s\n", $$1, $$2}'

migrate-up: ## Run all pending migrations (the server also does this on startup)
	@echo "Running migrations on $(DB_PATH)..."
	@go run cmd/migrate/main.go up

migrate-down: ## Roll back exactly ONE migration -- destructive, see warning below
	@echo "About to roll back ONE migration on $(DB_PATH)."
	@echo "This runs that migration's .down.sql and DESTROYS the data in whatever"
	@echo "it drops. There is no undo. Back the database file up first."
	@printf "Type 'yes' to continue: " && read ans && [ "$$ans" = "yes" ]
	@go run cmd/migrate/main.go down

migrate-create: ## Create a new migration file (usage: make migrate-create NAME=create_users_table)
	@if [ -z "$(NAME)" ]; then \
		echo "Error: NAME is required. Usage: make migrate-create NAME=your_migration_name"; \
		exit 1; \
	fi
	@migrate create -ext sql -dir $(MIGRATIONS_DIR) -seq $(NAME)
	@echo "Migration files created in $(MIGRATIONS_DIR)!"

migrate-status: ## Show current migration version
	@echo "Current migration status for $(DB_PATH):"
	@go run cmd/migrate/main.go version

migrate-force: ## Force set migration version (usage: make migrate-force VERSION=1)
	@if [ -z "$(VERSION)" ]; then \
		echo "Error: VERSION is required. Usage: make migrate-force VERSION=1"; \
		exit 1; \
	fi
	@echo "Forcing migration version to $(VERSION) on $(DB_PATH)..."
	@migrate -path $(MIGRATIONS_DIR) -database "sqlite3://$(DB_PATH)" force $(VERSION)
	@echo "Migration version forced to $(VERSION)!"

migrate-version: ## Show available migration files
	@echo "Available migration files:"
	@ls -1 $(MIGRATIONS_DIR)/*.sql 2>/dev/null || echo "No migration files found"

# Version stamping. /health serves these so a bug report can be tied to the
# binary that produced it. Override APP_VERSION on a release build; the commit
# and date are derived from the working tree. A bare `go build` still reports a
# usable commit via Go's embedded VCS stamp (see buildinfo.Get).
APP_VERSION ?= dev
APP_COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null)
APP_BUILD_DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS = -X mycorrhizal/buildinfo.Version=$(APP_VERSION) \
          -X mycorrhizal/buildinfo.Commit=$(APP_COMMIT) \
          -X mycorrhizal/buildinfo.BuildDate=$(APP_BUILD_DATE)

# Development helpers
dev: ## Build and run the server in development mode
	@go build -ldflags "$(LDFLAGS)" -o mycorrhizal . && ./mycorrhizal

test: ## Run all tests
	@go test ./... -v

build: ## Build the application
	@go build -ldflags "$(LDFLAGS)" -o mycorrhizal .

clean: ## Clean build artifacts
	@rm -f mycorrhizal
	@echo "Build artifacts cleaned!"
