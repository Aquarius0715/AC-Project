# syntax=docker/dockerfile:1.7
# AC Project web apps (Next.js App Router): one image per entry point, built from the npm workspace in service/
# (IR178). Build with --build-arg APP=customer | partner | technician | admin; context service/web/.
ARG NODE_VERSION=26.12.1

FROM node:${NODE_VERSION}-bookworm-slim AS deps
WORKDIR /src
COPY package.json package-lock.json ./
COPY shared/package.json shared/
COPY customer/package.json customer/
COPY partner/package.json partner/
COPY technician/package.json technician/
COPY admin/package.json admin/
RUN --mount=type=cache,target=/root/.npm npm ci

FROM node:${NODE_VERSION}-bookworm-slim AS build
ARG APP
WORKDIR /src
ENV NEXT_TELEMETRY_DISABLED=1
COPY --from=deps /src ./
COPY shared shared
COPY ${APP} ${APP}
RUN npm run typecheck -w ${APP} && npm run build -w ${APP}

# The runtime is the official Node image, not distroless: distroless's Node 22 line stopped at 22.22.0 without the fixes
# of 22.23.x (docker scout, IR408), and the web now runs on Node 26 (IR441). The server runs as the image's non-root `node` user; npm, npx and corepack are removed,
# as the server only needs node. The Debian packages are upgraded to their security fixes (docker scout, IR408).
FROM node:${NODE_VERSION}-bookworm-slim AS runtime
ARG APP
WORKDIR /app
ENV NODE_ENV=production NEXT_TELEMETRY_DISABLED=1 PORT=3000 HOSTNAME=0.0.0.0
RUN apt-get update && apt-get upgrade -y && apt-get clean && rm -rf /var/lib/apt/lists/* \
 && rm -rf /usr/local/lib/node_modules/npm /usr/local/lib/node_modules/corepack /usr/local/bin/npm /usr/local/bin/npx /usr/local/bin/corepack
# standalone output keeps the workspace layout (outputFileTracingRoot = service/web/)
COPY --from=build --chown=node:node /src/${APP}/.next/standalone ./
COPY --from=build --chown=node:node /src/${APP}/.next/static ./${APP}/.next/static
WORKDIR /app/${APP}
USER node
EXPOSE 3000
CMD ["node", "server.js"]
