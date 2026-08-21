# Build stage
FROM --platform=$BUILDPLATFORM node:26-alpine@sha256:aadf416b2cdce311a8811ba3f0608a61b77dbf997500e2eafe781b51f6a0b019 AS builder

WORKDIR /app

# node:26-alpine no longer bundles Corepack/Yarn (Corepack was dropped from the
# default Node.js distribution), so the classic Yarn CLI has to be installed
# explicitly to honor yarn.lock (lockfile v1).
#
# A plain `npm install -g yarn@<version>` is a semver pin, not a hash pin --
# OSSF Scorecard's Pinned-Dependencies check only credits `npm ci` or a
# git+https URL pinned to a 40-char commit SHA, and yarnpkg/yarn's git tags
# don't include the built lib/ directory (it's produced by `gulp build`,
# gitignored, only shipped in the published npm tarball), so a git-SHA
# install doesn't actually work. Download the exact release tarball instead
# and verify it against its published sha512 (cross-checked against the
# registry's `dist.integrity` for yarn@1.22.22) before installing.
RUN wget -q https://registry.npmjs.org/yarn/-/yarn-1.22.22.tgz -O /tmp/yarn.tgz && \
    echo "a6b2f7906b721bba3d67d4aff083df04dad64c399707841b7acf00f6b133b7ac24255f2652fa22ae3534329dc6180534e98d17432037ff6fd140556e2bb3137e  /tmp/yarn.tgz" | sha512sum -c - && \
    npm install -g /tmp/yarn.tgz && \
    rm /tmp/yarn.tgz

# Copy package files first for better caching
COPY package.json yarn.lock ./

# Install dependencies. The repo is yarn-only (yarn.lock is committed, no
# package-lock.json), so this is always the frozen-lockfile path. The previous
# `npm ci` / `npm install` fallbacks were dead code, and OSSF Scorecard flagged
# the `npm install` branch as an unpinned npm command.
RUN yarn install --frozen-lockfile

# Copy source code
COPY . .

# Build the app with the API URL injected at build time
ARG VITE_API_URL
ENV VITE_API_URL=${VITE_API_URL}

RUN if [ -f yarn.lock ]; then yarn build; else npm run build; fi

# Production stage - serve with unprivileged nginx
FROM nginxinc/nginx-unprivileged:1.30-alpine@sha256:44e36330f74d4f3a1d4e222acca9e23b401fb87811a7597024502bb759c4dd49

# Copy custom nginx config
COPY nginx.conf /etc/nginx/conf.d/default.conf

# Copy built assets from builder
COPY --from=builder /app/build /usr/share/nginx/html

# nginx-unprivileged runs on port 8080 by default
EXPOSE 8080

# Start nginx
CMD ["nginx", "-g", "daemon off;"]
