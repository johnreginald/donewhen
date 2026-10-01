.PHONY: build run web web-dev migrate token genvapid test up down logs engine backup clean tidy

# bash: the backup recipe needs pipefail so a failed pg_dump is not hidden by gzip.
SHELL := bash

BIN := ./raenil

# Container engine. Podman on the dev Mac, Docker on the production box — both
# read compose.yaml. Auto-detected; override explicitly when you need to:
#   make up COMPOSE="docker compose"
COMPOSE ?= $(shell command -v podman >/dev/null 2>&1 && echo "podman compose" || echo "docker compose")

# Optional compose profile, e.g. make up PROFILE=edge
PROFILE_ARG := $(if $(PROFILE),--profile $(PROFILE),)

build: ## build the Go binary
	go build -o $(BIN) ./cmd/raenil

web: ## build the SvelteKit frontend into web/build
	cd web && npm install && npm run build

web-dev: ## run the Vite dev server (proxies /api to :8080)
	cd web && npm install && npm run dev

run: build ## run the server locally
	$(BIN) serve

migrate: build ## apply DB migrations; backs up first if a destructive one is pending
	@pending=$$($(BIN) migrate-pending) || exit 1; \
	if [ -n "$$pending" ]; then \
		echo "destructive migration pending ($$pending): taking a backup first"; \
		$(MAKE) backup || exit 1; \
	fi; \
	RAENIL_BACKUP_CONFIRMED="$$pending" $(BIN) migrate

user: build ## create initial user: make user EMAIL=you@x.com PASS=secret123
	$(BIN) user $(EMAIL) $(PASS)

token: build ## create an API token: make token NAME=claude
	$(BIN) token $(NAME)

genvapid: build ## print a fresh VAPID keypair
	$(BIN) genvapid

test: ## run Go tests
	go test ./...

tidy: ## tidy modules
	go mod tidy

up: ## compose up (build + run all)
	$(COMPOSE) $(PROFILE_ARG) up -d --build

down: ## compose down
	$(COMPOSE) $(PROFILE_ARG) down

logs: ## tail compose logs
	$(COMPOSE) logs -f --tail=100

engine: ## show which container engine will be used
	@echo "COMPOSE = $(COMPOSE)"

backup: ## dump the database to ./backups
	@mkdir -p backups
	@set -o pipefail; f=backups/raenil-$$(date +%Y%m%d-%H%M%S).sql.gz; \
	if $(COMPOSE) exec -T db pg_dump -U $${POSTGRES_USER:-raenil} $${POSTGRES_DB:-raenil} | gzip > $$f; then \
		echo "backup written to $$f"; \
	else rm -f $$f; echo "backup FAILED" >&2; exit 1; fi

clean:
	rm -f $(BIN)
