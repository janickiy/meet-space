FROM --platform=$BUILDPLATFORM golang:1.26.10-alpine3.23 AS build
ARG TARGETARCH
ARG GOMAXPROCS
ARG GOFLAGS
ARG GOGC
ARG GOMEMLIMIT

WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,id=meetspace-go126-modules-${TARGETARCH},target=/go/pkg/mod,sharing=locked \
    go mod download
COPY cmd/ ./cmd/
COPY internal/ ./internal/
COPY tools/ ./tools/
ARG BUILD_VERSION=dev
ARG BUILD_COMMIT=unknown
ARG BUILD_TIME=unknown
RUN --mount=type=cache,id=meetspace-go126-modules-${TARGETARCH},target=/go/pkg/mod,sharing=locked \
    --mount=type=cache,id=meetspace-go126-build-${TARGETARCH},target=/root/.cache/go-build,sharing=locked \
    CGO_ENABLED=0 GOOS=linux GOARCH=$TARGETARCH go build -trimpath \
    -ldflags "-s -w -X github.com/janickiy/meet-space/internal/buildinfo.Version=$BUILD_VERSION -X github.com/janickiy/meet-space/internal/buildinfo.Commit=$BUILD_COMMIT -X github.com/janickiy/meet-space/internal/buildinfo.BuildTime=$BUILD_TIME" \
    -o /bin/media-worker ./cmd/media-worker

FROM alpine:3.22 AS runtime

RUN apk add --no-cache ca-certificates \
    && addgroup -S app && adduser -S app -G app
WORKDIR /app
COPY --from=build /bin/media-worker /app/media-worker
USER app
EXPOSE 8091
ENTRYPOINT ["/app/media-worker"]
