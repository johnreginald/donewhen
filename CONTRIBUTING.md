# Contributing to DoneWhen

Thanks for your help. DoneWhen is a self-hosted issue tracker. It has a Go backend, a SvelteKit frontend, and Postgres.

## Set up

You need:

- Go (the version in `go.mod`)
- Node 22 and npm
- Docker, for Postgres

```sh
git clone https://github.com/johnreginald/donewhen.git
cd donewhen

# Postgres for development and tests
docker run -d --name donewhen-dev-pg -p 55433:5432 \
  -e POSTGRES_USER=donewhen -e POSTGRES_PASSWORD=donewhen -e POSTGRES_DB=donewhen \
  postgres:17

# Frontend
cd web && npm ci && cd ..
```

To run the app, use `make web` (it builds the frontend), then `make run`. Use `make web-dev` for the Vite dev server.

## Run the tests

Run all three checks before you open a PR.

```sh
# Go: starts a throwaway Postgres in Docker, runs `go vet` and `go test ./...`, then removes it
make test

# Web: the build runs check:ds first
cd web && npm run build

# Plugin
node --test 'plugin/scripts/*.test.mjs'
```

`make test` needs Docker. It uses its own container on a free local port, so it never touches your dev database. It sets `DONEWHEN_TEST_STRICT=1`. A database test that cannot find Postgres then fails and does not skip.

For workspace-page changes, also run the browser regression tests. They build
the frontend, start a local preview, and mock every API request, so no running
backend or real workspace data is needed:

```sh
cd web
npx playwright install chromium # once
npm run test:browser
```

To use an existing Chrome installation, set `PLAYWRIGHT_EXECUTABLE_PATH` instead.

To run one package against your own database, set `DONEWHEN_TEST_DATABASE_URL` and run `go test ./internal/api`. Without `DONEWHEN_TEST_STRICT`, those tests skip when the variable is not set.

Also run `gofmt -l .` (it must print nothing). `make test` already runs `go vet ./...`. There is no CI, so run these checks before you open a PR.

## Design system

Use only the tokens defined in `web/src/app.css` (colours, spacing, radii, type). Do not hard-code values.
`npm run check:ds` enforces this rule, and `npm run build` runs it.

## Branches and commits

- Branch from `main`. Name the branch by type: `feat/...`, `fix/...`, `chore/...`.
- Write a short commit subject in the imperative. Do not add a full stop.
- In the commit body, say why the change is needed. Do not only say what it does.
- Keep one idea in each commit.

## Pull requests

1. Push your branch to your fork and open a PR against `main`.
2. Fill in the PR template.
3. Keep the PR small and focused. Explain the reason for the change.
4. Run the checks above first. A maintainer reviews every PR.

## Security issues

Do not open a public issue. See [SECURITY.md](SECURITY.md).

## Licence

DoneWhen is licensed under the [GNU AGPL-3.0](LICENSE). When you contribute, you agree that your contributions are licensed under AGPL-3.0.

## Code of conduct

Be kind. See [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md).
