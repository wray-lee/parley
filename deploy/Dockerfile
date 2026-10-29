# Server Host image: one static Go binary on distroless (ADR-0003). The binary
# embeds config/, packs/, prompts/, samples/ and the vanilla web client, so the
# runtime image is just the binary and CA certificates.
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
USER nonroot:nonroot
EXPOSE 3000
ENTRYPOINT ["/app/parley-server"]
