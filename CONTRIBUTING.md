# Contributing to DoneWhen

Thanks for helping. DoneWhen is a self-hosted issue tracker: Go backend, SvelteKit frontend, Postgres.

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

Run the app with `make web` (builds the frontend), then `make run`. Use `make web-dev` for the Vite dev server.

## Run the tests

Run all three before you open a PR.

```sh
# Go: starts a throwaway Postgres in Docker, runs `go vet` and `go test ./...`, then removes it
make test

# Web: the build runs check:ds first
cd web && npm run build

# Plugin
node --test 'plugin/scripts/*.test.mjs'
```

`make test` needs Docker and uses its own container on a free local port, so it never touches your dev database. It sets `DONEWHEN_TEST_STRICT=1`, so a database test that cannot find Postgres fails instead of skipping. To run one package against your own database, set `DONEWHEN_TEST_DATABASE_URL` and run `go test ./internal/api` (without `DONEWHEN_TEST_STRICT`, those tests skip when it is unset).

Also run `gofmt -l .` (it must print nothing); `make test` already runs `go vet ./...`. There is no CI: run these checks before you open a PR.

## Design system

Use only the tokens defined in `web/src/app.css` (colours, spacing, radii, type). Do not hard-code values.
`npm run check:ds` enforces this, and `npm run build` runs it.

## Branches and commits

- Branch from `main`. Name it by type: `feat/...`, `fix/...`, `chore/...`.
- Commit subject: short, imperative, no full stop.
- Commit body: say why the change is needed, not only what it does.
- Keep one idea per commit.

## Pull requests

1. Push your branch to your fork and open a PR against `main`.
2. Fill in the PR template.
3. Keep the PR small and focused. Explain the why.
4. Run the checks above first. A maintainer reviews every PR.

## Security issues

Do not open a public issue. See [SECURITY.md](SECURITY.md).

## Licence

DoneWhen is licensed under the [GNU AGPL-3.0](LICENSE). By contributing, you agree that your contributions are licensed under AGPL-3.0.

## Code of conduct

Be kind. See [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md).
