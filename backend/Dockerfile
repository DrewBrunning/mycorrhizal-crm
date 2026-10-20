# Build stage
FROM --platform=$BUILDPLATFORM golang:1.27.2-alpine@sha256:85dc1069ac644ea3c527b177303a406eb3358192816cd7f9e5848eb658851673 AS builder

ARG TARGETOS
ARG TARGETARCH

WORKDIR /app

# Copy go mod files first for better caching
COPY go.mod go.sum ./
# Retry the module download: proxy.golang.org intermittently drops the
# connection mid-stream and `go mod download` fails hard on it (a nightly
# Docker-build flake, issue #1461). One transient stream error must not fail
# the whole image build; the retries reuse whatever was already fetched.
RUN go mod download || (sleep 5 && go mod download) || (sleep 15 && go mod download)

# Copy source code
COPY . .

# Version stamping, same as the all-in-one image at /Dockerfile. This is a
# separately published image, so it needs its own stamp -- without it /health
# reports "dev" and a bug report cannot be tied to a build.
ARG APP_VERSION=dev
ARG APP_COMMIT=""
ARG APP_BUILD_DATE=""

# Build the application. -trimpath drops the builder's absolute paths and
# -buildvcs=false keeps VCS state out, so the binary is byte-identical across
# machines -- the commit comes from the -X ldflag (REL-04, issue #448;
# docs/security/reproducible-builds.md).
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build \
    -trimpath -buildvcs=false \
    -ldflags "-X mycorrhizal/buildinfo.Version=${APP_VERSION} \
              -X mycorrhizal/buildinfo.Commit=${APP_COMMIT} \
              -X mycorrhizal/buildinfo.BuildDate=${APP_BUILD_DATE}" \
    -o mycorrhizal .

# Runtime stage
FROM alpine:3.24@sha256:294b683cb724975bec92580e1e685676bd4b50bda910ddb8c51d4cabeaec77e6

# Install runtime dependencies (versions pinned per hadolint DL3018 against
# the alpine:3.24 index).
#
# libssl3/libcrypto3 pinned explicitly, above what the base image ships
# (3.5.7-r0), to pull in the CVE-2026-14456 fix (3.5.8-r0) already present in
# this same pinned index -- caught by Trivy gating v0.6.1's release scan
# (exit-code: 1 on fixed CRITICAL/HIGH). No base-image digest bump needed;
# `apk policy libssl3` against this exact digest already lists 3.5.8-r0 from
# alpine/v3.24/main.
#
# 2026-09-30: bumped 3.5.8-r0 -> 3.5.9-r0. Alpine published 3.5.9-r0 to
# alpine/v3.24/main and pruned 3.5.8-r0 from the index, so the old pin became
# unsatisfiable (`apk add` exit 3) -- the same pruned-pin class as #1062/#1131.
# 2026-10-14 (issue #1703): zlib pinned explicitly, above the 1.3.2-r0 the
# base image ships, for CVE-2026-85091 (heap overflow, fixed 1.3.2-r1) -- caught
# by the v1.4.2 publish scan. The pinned alpine:3.24 digest is still the
# current one and still carries r0, so no digest bump helps; r1 is in the same
# alpine/v3.24/main index. Exact pin, like the rest of this list (DL3018); if
# Alpine prunes 1.3.2-r1 the build fails loudly and the pin is bumped, as with
# libssl3 above (#1062/#1131).
# 2026-10-20: bumped ca-certificates 20260909-r0 -> 20261020-r0. Alpine
# published 20261020-r0 and pruned the old one from alpine/v3.24/main, so the
# old pin became unsatisfiable (`apk add`: "breaks: world[...]") -- the same
# pruned-pin class as libssl3 above.
RUN apk add --no-cache ca-certificates=20261020-r0 tzdata=2026e-r0 shadow=4.18.0-r1 su-exec=0.3-r0 libc6-compat=1.1.0-r4 libssl3=3.5.9-r0 libcrypto3=3.5.9-r0 zlib=1.3.2-r1

WORKDIR /app

# Create non-root user for security
RUN addgroup -g 1001 -S appgroup && adduser -u 1001 -S appuser -G appgroup

# Create directories for data persistence
RUN mkdir -p /app/static/photos /app/data && \
    chown -R appuser:appgroup /app

# Set default environment
ENV PORT=8080
ENV SQLITE_DB_PATH=/app/data/mycorrhizal.db
ENV PROFILE_PHOTO_DIR=/app/static/photos

# Copy the binary from builder
COPY --from=builder /app/mycorrhizal .

# Copy static assets if any
COPY --from=builder /app/static/styles.css ./static/

# Copy entrypoint script
COPY entrypoint.sh /app/entrypoint.sh
RUN chmod +x /app/entrypoint.sh

# Expose the port
EXPOSE 8080

# Issue #578: the split backend image is a published artifact (docker-publish
# matrix) and must carry a HEALTHCHECK like the all-in-one does — trivy's
# DS-0026 flagged its absence once the misconfig gate hard-gated. The Go
# server serves the health surface directly on 8080 (backend/routes/routes.go),
# no auth. Probe /health/live (liveness, issue #421): a restart policy wants
# "is the process up", not the deep check or readiness.
HEALTHCHECK --interval=30s --timeout=10s --start-period=10s --retries=3 \
    CMD ["wget", "--no-verbose", "--tries=1", "--spider", "http://127.0.0.1:8080/health/live"]

# Run as root initially; entrypoint drops to appuser after adjusting UID/GID
ENTRYPOINT ["/app/entrypoint.sh"]
CMD ["./mycorrhizal"]
