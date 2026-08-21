# Build stage
FROM --platform=$BUILDPLATFORM node:26-alpine@sha256:aadf416b2cdce311a8811ba3f0608a61b77dbf997500e2eafe781b51f6a0b019 AS builder

WORKDIR /app

# node:26-alpine no longer bundles Corepack/Yarn (Corepack was dropped from the
# default Node.js distribution), so the classic Yarn CLI has to be installed
# explicitly to honor yarn.lock (lockfile v1).
RUN npm install -g yarn

# Copy package files first for better caching
COPY package.json yarn.lock* package-lock.json* ./

# Install dependencies
RUN if [ -f yarn.lock ]; then yarn install --frozen-lockfile; \
    elif [ -f package-lock.json ]; then npm ci; \
    else npm install; fi

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
