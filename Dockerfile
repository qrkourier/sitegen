# syntax=docker/dockerfile:1
FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download
COPY . .
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /sitegen .

FROM alpine:3.21
# hadolint ignore=DL3018
RUN apk add --no-cache ca-certificates
COPY --from=build /sitegen /usr/local/bin/sitegen
ENTRYPOINT ["sitegen"]
CMD ["serve", "-addr", ":8080"]
EXPOSE 8080
