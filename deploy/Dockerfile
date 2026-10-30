# Server Host image: one static Go binary on distroless (ADR-0003). The binary
# embeds config/, packs/, prompts/, samples/ and the vanilla web client; the
# Flutter web bundle is copied to web-app/ and served at / (vanilla stays at
# /lite). The release workflow extracts parley-*-web.tar.gz there before the
# build; a fresh clone without it still serves the vanilla client at /.
FROM golang:1.26.5-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/parley-server ./cmd/parley-server

FROM gcr.io/distroless/static-debian12:nonroot

WORKDIR /app
ENV PORT=3000
COPY --from=build /out/parley-server /app/parley-server
COPY --from=build /src/web-app /app/web-app
USER nonroot:nonroot
EXPOSE 3000
ENTRYPOINT ["/app/parley-server"]
