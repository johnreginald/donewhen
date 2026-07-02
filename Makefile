.PHONY: build run web web-dev migrate token genvapid test up down logs backup clean tidy

BIN := ./kanri

build: ## build the Go binary
	go build -o $(BIN) ./cmd/kanri

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

up: ## docker-compose up (build + run all)
	docker compose up -d --build

down: ## docker-compose down
	docker compose down

logs: ## tail compose logs
	docker compose logs -f --tail=100

backup: ## dump the database to ./backups
	@mkdir -p backups
	docker compose exec -T db pg_dump -U $${POSTGRES_USER:-kanri} $${POSTGRES_DB:-kanri} | gzip > backups/kanri-$$(date +%Y%m%d-%H%M%S).sql.gz
	@echo "backup written to ./backups"

clean:
	rm -f $(BIN)
