# ---- stage 1: build the SvelteKit frontend ----
FROM node:22-alpine AS web
WORKDIR /web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

# ---- stage 2: build the Go binary ----
FROM golang:1.25 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG GIT_COMMIT=dev
ARG BUILD_TIME=
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags "-X github.com/johnreginald/donewhen/internal/version.Commit=${GIT_COMMIT} -X github.com/johnreginald/donewhen/internal/version.BuiltAt=${BUILD_TIME}" \
    -o /out/donewhen ./cmd/donewhen

# ---- stage 3: minimal runtime ----
FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata
# Run as an unprivileged user; the app writes nothing to disk.
RUN addgroup -S -g 10001 donewhen && adduser -S -u 10001 -G donewhen -h /app donewhen
WORKDIR /app
COPY --from=build /out/donewhen /app/donewhen
COPY --from=web /web/build /app/web/build
USER donewhen
EXPOSE 8080
# /api/health is unauthenticated. The port follows DONEWHEN_LISTEN_ADDR (default :8080).
HEALTHCHECK --interval=30s --timeout=5s --start-period=40s --retries=3 \
  CMD sh -c 'a="${DONEWHEN_LISTEN_ADDR:-:8080}"; wget -q -O /dev/null "http://127.0.0.1:${a##*:}/api/health" || exit 1'
ENTRYPOINT ["/app/donewhen"]
CMD ["serve"]
