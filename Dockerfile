# ---- stage 1: build the SvelteKit frontend ----
FROM node:22-alpine AS web
WORKDIR /web
COPY web/package.json web/package-lock.json* ./
RUN npm install
COPY web/ ./
RUN npm run build

# ---- stage 2: build the Go binary ----
FROM golang:1.25 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/raenil ./cmd/raenil

# ---- stage 3: minimal runtime ----
FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=build /out/raenil /app/raenil
COPY --from=web /web/build /app/web/build
EXPOSE 8080
ENTRYPOINT ["/app/raenil"]
CMD ["serve"]
