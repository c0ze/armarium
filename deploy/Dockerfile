# Armarium: one static binary on scratch, built for amd64 and arm64.
#   docker build -t armarium -f deploy/Dockerfile .
#   docker buildx build --platform linux/amd64,linux/arm64 -f deploy/Dockerfile .
# The build stages run on the build machine's platform and cross-compile, so a
# multi-arch build needs no emulation.
FROM --platform=$BUILDPLATFORM node:24-alpine AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY web/ ./
RUN npm run build

FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/web/dist ./web/dist
ARG VERSION=docker
ARG TARGETOS TARGETARCH
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /armarium ./cmd/armarium \
 && mkdir -p /data && chown 65532:65532 /data

FROM scratch
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=build /armarium /armarium
COPY --from=build --chown=65532:65532 /data /data
# Non-root by default. On a NAS, run as the user that owns the data folder
# (Synology: `user: "1026:100"` in compose, or your own id from `id`).
USER 65532:65532
ENV ARMARIUM_DATA_DIR=/data ARMARIUM_LISTEN=0.0.0.0:8580 ARMARIUM_CONFIG=/config/armarium.toml
EXPOSE 8580
VOLUME ["/data"]
HEALTHCHECK --interval=5m --timeout=5s CMD ["/armarium", "healthcheck"]
ENTRYPOINT ["/armarium"]
CMD ["serve"]
