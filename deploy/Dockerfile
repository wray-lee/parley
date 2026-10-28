# Server Host image: Bun runtime, no build step (ADR-0001).
FROM oven/bun:1.4.3 AS deps
WORKDIR /app
# Only production deps: the `ws` WebSocket library.
COPY package.json bun.lock ./
RUN bun install --production --frozen-lockfile

FROM oven/bun:1.4.3-alpine

WORKDIR /app
ENV NODE_ENV=production PORT=3000

COPY --from=deps /app/node_modules ./node_modules
COPY package.json ./

COPY core ./core
COPY config ./config
COPY hosts ./hosts
COPY web ./web
COPY prompts ./prompts
COPY packs ./packs
COPY samples ./samples

USER bun
EXPOSE 3000
CMD ["bun", "hosts/node/server.js"]
