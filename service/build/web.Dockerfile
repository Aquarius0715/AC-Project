# syntax=docker/dockerfile:1.7
# AC Project web apps (Next.js App Router): one image per entry point, built from the npm workspace in service/
# (IR178). Build with --build-arg APP=customer-web | partner-web | technician-web | admin-web; context service/.
ARG NODE_VERSION=22

FROM node:${NODE_VERSION}-bookworm-slim AS deps
WORKDIR /src
COPY package.json package-lock.json ./
COPY web-shared/package.json web-shared/
COPY customer-web/package.json customer-web/
COPY partner-web/package.json partner-web/
COPY technician-web/package.json technician-web/
COPY admin-web/package.json admin-web/
RUN --mount=type=cache,target=/root/.npm npm ci

FROM node:${NODE_VERSION}-bookworm-slim AS build
ARG APP
WORKDIR /src
ENV NEXT_TELEMETRY_DISABLED=1
COPY --from=deps /src ./
COPY web-shared web-shared
COPY ${APP} ${APP}
RUN npm run typecheck -w ${APP} && npm run build -w ${APP}

FROM gcr.io/distroless/nodejs${NODE_VERSION}-debian12:nonroot AS runtime
ARG APP
WORKDIR /app
ENV NODE_ENV=production NEXT_TELEMETRY_DISABLED=1 PORT=3000 HOSTNAME=0.0.0.0
# standalone output keeps the workspace layout (outputFileTracingRoot = service/)
COPY --from=build --chown=nonroot:nonroot /src/${APP}/.next/standalone ./
COPY --from=build --chown=nonroot:nonroot /src/${APP}/.next/static ./${APP}/.next/static
WORKDIR /app/${APP}
USER nonroot
EXPOSE 3000
CMD ["server.js"]
