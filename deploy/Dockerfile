# Server Host image: plain Node, no build step (ADR-0001).
FROM node:24-alpine

WORKDIR /app
ENV NODE_ENV=production PORT=3000

# Only production deps: the `ws` WebSocket library.
COPY package.json package-lock.json ./
RUN npm ci --omit=dev

COPY core ./core
COPY config ./config
COPY hosts ./hosts
COPY web ./web
COPY prompts ./prompts
COPY packs ./packs
COPY samples ./samples

USER node
EXPOSE 3000
CMD ["node", "hosts/node/server.js"]
