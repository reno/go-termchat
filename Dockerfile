FROM node:24.18.0-alpine AS web
WORKDIR /src
RUN corepack enable pnpm
COPY web/package.json web/pnpm-lock.yaml ./web/
RUN cd web && pnpm install --frozen-lockfile
COPY web ./web
RUN cd web && pnpm run build

FROM golang:1.26.6-alpine AS server
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/web/dist ./cmd/server/dist
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/termchat ./cmd/server

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=server /out/termchat /termchat
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/termchat"]
