# The factory: one image with every vendor's own agent CLI, the toolchains the
# checks need, and the orchestrator host that launches them. Workers run as an
# unprivileged user inside it, with no view of the owner's machine.
FROM golang:1.25 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 go build -trimpath -o /out/orchestrator ./cmd/orchestrator

FROM debian:bookworm-slim
ARG UID=1000
ARG GID=1000
RUN apt-get update && apt-get install -y --no-install-recommends \
      ca-certificates curl git make openssh-client gnupg jq unzip xz-utils procps less build-essential \
 && install -m 0755 -d /etc/apt/keyrings \
 && curl -fsSL https://cli.github.com/packages/githubcli-archive-keyring.gpg -o /etc/apt/keyrings/github.gpg \
 && echo "deb [signed-by=/etc/apt/keyrings/github.gpg] https://cli.github.com/packages stable main" > /etc/apt/sources.list.d/github.list \
 && curl -fsSL https://download.docker.com/linux/debian/gpg -o /etc/apt/keyrings/docker.asc \
 && echo "deb [signed-by=/etc/apt/keyrings/docker.asc] https://download.docker.com/linux/debian bookworm stable" > /etc/apt/sources.list.d/docker.list \
 && curl -fsSL https://deb.nodesource.com/setup_22.x | bash - \
 && apt-get install -y --no-install-recommends gh docker-ce-cli docker-compose-plugin nodejs \
 && rm -rf /var/lib/apt/lists/*

# Go for builds and tests, the same toolchain the orchestrator was built with.
COPY --from=build /usr/local/go /usr/local/go

# Each vendor's own harness.
RUN npm install -g @anthropic-ai/claude-code @openai/codex opencode-ai && npm cache clean --force

RUN groupadd -g ${GID} factory && useradd -m -u ${UID} -g ${GID} -s /bin/bash factory
USER factory
ENV HOME=/home/factory \
    PATH=/home/factory/.local/bin:/home/factory/go/bin:/usr/local/go/bin:/usr/local/bin:/usr/bin:/bin \
    GOPATH=/home/factory/go
# Antigravity's CLI installs per user.
RUN curl -fsSL https://antigravity.google/cli/install.sh | bash && (agy --version || true)

COPY --from=build /out/orchestrator /usr/local/bin/orchestrator
COPY --chmod=0755 deploy/factory/entrypoint.sh /usr/local/bin/factory-entrypoint
ENTRYPOINT ["factory-entrypoint"]
