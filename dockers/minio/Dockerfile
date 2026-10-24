FROM golang:1.26.10-alpine3.23 AS build
ARG GOMAXPROCS
ARG GOFLAGS=-p=2
ARG GOGC
ARG GOMEMLIMIT

# Исходные образы сообщества больше недоступны в реестрах.
# Собираем закреплённый официальный релиз из исходников вместо использования стороннего образа.
ARG MINIO_VERSION=RELEASE.2025-10-15T17-29-55Z
RUN CGO_ENABLED=0 GOBIN=/out go install -trimpath -ldflags="-s -w" github.com/minio/minio@${MINIO_VERSION}

FROM alpine:3.22 AS runtime

RUN apk add --no-cache ca-certificates tzdata
COPY --from=build /out/minio /usr/local/bin/minio

EXPOSE 9000 9001
ENTRYPOINT ["/usr/local/bin/minio"]
