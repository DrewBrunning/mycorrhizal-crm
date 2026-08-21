# Mycorrhizal CRM - All-in-one image
#
# Bundles React frontend and the Go backend into a single container, served by nginx
#
# Build context is the repository root:
#   docker build -t mycorrhizal-crm .

# =============================================================================
# Stage 1: Build Go backend (cross-compiled, CGO disabled - pure-Go SQLite)
# =============================================================================
FROM --platform=$BUILDPLATFORM golang:1.26.6-alpine AS backend-builder

ARG TARGETOS
ARG TARGETARCH

WORKDIR /app

# Copy go mod files first for better caching
COPY backend/go.mod backend/go.sum ./
RUN go mod download

# Copy backend source code
COPY backend/ .

# Version stamping. Passed as build args so the image records which source it
# was built from -- /health serves these, and they are what ties a user's bug
# report to a specific binary. Without them the binary reports "dev".
ARG APP_VERSION=dev
ARG APP_COMMIT=""
ARG APP_BUILD_DATE=""

# Build the application (glebarez/sqlite is pure Go (no CGO, i.e. without QEMU)
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build \
    -ldflags "-X mycorrhizal/buildinfo.Version=${APP_VERSION} \
              -X mycorrhizal/buildinfo.Commit=${APP_COMMIT} \
              -X mycorrhizal/buildinfo.BuildDate=${APP_BUILD_DATE}" \
    -o mycorrhizal .

# =============================================================================
# Stage 2: Build React frontend (same-origin API, served by nginx)
# =============================================================================
# react-router 8 requires Node >=22.22.0 (engines field). Matches CI's own
# node-version: 22 pin (unit-tests.yml, e2e-tests.yml) rather than floating on
# node:lts-alpine the way frontend/Dockerfile (the split image) does -- this
# repo prefers an explicit, reproducible pin over a floating tag (see the Go
# toolchain note in CLAUDE.md). frontend/Dockerfile didn't need this change:
# lts-alpine already resolves past the floor (Node 24 as of 2026-08).
FROM --platform=$BUILDPLATFORM node:22-alpine AS frontend-builder

WORKDIR /app

# Copy package files first for better caching
COPY frontend/package.json frontend/yarn.lock* frontend/package-lock.json* ./

# Install dependencies
RUN if [ -f yarn.lock ]; then yarn install --frozen-lockfile; \
    elif [ -f package-lock.json ]; then npm ci; \
    else npm install; fi

# Copy frontend source code
COPY frontend/ .

# API is served from the same origin; nginx proxies /api to the backend.
# Must stay empty so the bundle uses relative URLs
ENV VITE_API_URL=""

RUN if [ -f yarn.lock ]; then yarn build; else npm run build; fi

# =============================================================================
# Stage 3: Runtime - nginx + Go backend under supervisord
# =============================================================================
FROM alpine:3.24

# Runtime dependencies. shadow provides usermod/groupmod for PUID/PGID remap.
# No sqlite package needed - the backend uses a pure-Go SQLite driver.
RUN apk add --no-cache \
    ca-certificates \
    tzdata \
    nginx \
    supervisor \
    shadow \
    libc6-compat

WORKDIR /app

# Non-root user the backend process runs as
RUN addgroup -g 1001 -S appgroup && adduser -u 1001 -S appuser -G appgroup

# Runtime directories
RUN mkdir -p /app/data /app/static/photos /app/static/attachments /var/log/supervisor /run/nginx && \
    chown -R appuser:appgroup /app/data /app/static/photos /app/static/attachments

# Copy Go binary and static assets from the backend builder
COPY --from=backend-builder /app/mycorrhizal /app/mycorrhizal
COPY --from=backend-builder /app/static/styles.css /app/static/

# Copy frontend build to nginx html directory
COPY --from=frontend-builder /app/build /usr/share/nginx/html

# Configuration files
COPY docker/nginx.conf /etc/nginx/http.d/default.conf
COPY docker/supervisord.conf /etc/supervisor/conf.d/supervisord.conf
COPY docker/entrypoint.sh /app/entrypoint.sh
RUN chmod +x /app/entrypoint.sh

# Default environment
# PORT is the backend's internal bind port - nginx listens on 8080 (below) and
# proxies to it, so it must not collide with nginx's own port.
ENV PORT=8081
ENV SQLITE_DB_PATH=/app/data/mycorrhizal.db
ENV PROFILE_PHOTO_DIR=/app/static/photos
ENV ATTACHMENTS_DIR=/app/static/attachments
ENV GIN_MODE=release

# nginx listens on 8080 (no root needed to bind)
EXPOSE 8080

# Health check hits nginx, which proxies /health to the backend
HEALTHCHECK --interval=30s --timeout=10s --start-period=10s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://127.0.0.1:8080/health || exit 1

# Entrypoint remaps PUID/PGID + chowns data dirs, then launches supervisord
ENTRYPOINT ["/app/entrypoint.sh"]
CMD ["supervisord", "-c", "/etc/supervisor/conf.d/supervisord.conf"]
