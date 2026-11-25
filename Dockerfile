FROM node:22-bookworm-slim AS web
WORKDIR /src
RUN corepack enable
COPY web/package.json web/pnpm-lock.yaml ./web/
RUN pnpm --dir web install --frozen-lockfile
COPY web ./web
RUN pnpm --dir web build

FROM golang:1.26-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/web/dist ./web/dist
RUN CGO_ENABLED=0 go build -trimpath -o /out/watchover-tg .

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/watchover-tg /watchover-tg
ENV RUN_MODE=release HTTP_PORT=8080
EXPOSE 8080
ENTRYPOINT ["/watchover-tg"]
