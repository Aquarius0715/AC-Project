# syntax=docker/dockerfile:1.7
# AC Project web apps (Next.js App Router): one image per entry point, built from the npm workspace in service/
# (IR178). Build with --build-arg APP=customer | partner | technician | admin; context service/web/.
ARG NODE_VERSION=22

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

FROM gcr.io/distroless/nodejs${NODE_VERSION}-debian12:nonroot AS runtime
ARG APP
WORKDIR /app
ENV NODE_ENV=production NEXT_TELEMETRY_DISABLED=1 PORT=3000 HOSTNAME=0.0.0.0
# standalone output keeps the workspace layout (outputFileTracingRoot = service/web/)
COPY --from=build --chown=nonroot:nonroot /src/${APP}/.next/standalone ./
COPY --from=build --chown=nonroot:nonroot /src/${APP}/.next/static ./${APP}/.next/static
WORKDIR /app/${APP}
USER nonroot
EXPOSE 3000
CMD ["server.js"]
