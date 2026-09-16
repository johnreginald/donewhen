.PHONY: build run web web-dev migrate token genvapid test up down logs engine backup clean tidy

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

migrate: build ## apply DB migrations
	$(BIN) migrate

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
	$(COMPOSE) exec -T db pg_dump -U $${POSTGRES_USER:-raenil} $${POSTGRES_DB:-raenil} | gzip > backups/raenil-$$(date +%Y%m%d-%H%M%S).sql.gz
	@echo "backup written to ./backups"

clean:
	rm -f $(BIN)
