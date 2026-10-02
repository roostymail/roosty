# syntax=docker/dockerfile:1
# Roosty Mail: one static binary with the web app embedded.

FROM node:22-alpine AS web
WORKDIR /web
COPY web/package.json web/package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY web/ ./
RUN npm run build

FROM golang:1.27-alpine AS server
WORKDIR /src
COPY server/go.mod server/go.sum ./
RUN go mod download
COPY server/ ./
COPY --from=web /web/dist ./internal/web/dist
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/roosty ./cmd/roosty \
 && mkdir -p /out/data

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=server /out/roosty /roosty
COPY --from=server --chown=65532:65532 /out/data /data
ENV ROOSTY_DATA_DIR=/data ROOSTY_LISTEN=:8080
VOLUME /data
EXPOSE 8080
USER nonroot
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s CMD ["/roosty", "healthcheck"]
ENTRYPOINT ["/roosty"]
