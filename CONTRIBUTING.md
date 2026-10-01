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
# Go (needs the database above)
DONEWHEN_TEST_DATABASE_URL='postgres://donewhen:donewhen@localhost:55433/donewhen?sslmode=disable' go test ./...

# Web: the build runs check:ds first
cd web && npm run build

# Plugin
node --test 'plugin/scripts/*.test.mjs'
```

Also run `go vet ./...` and `gofmt -l .` (it must print nothing). There is no CI: run these checks before you open a PR.

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
