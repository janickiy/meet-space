FROM scratch AS seaweedfs-source
ADD --checksum=sha256:93199495e93a981451d1fefecb5aa71e12aa1692093eed2338bbfe7f08960bfb https://codeload.github.com/seaweedfs/seaweedfs/tar.gz/refs/tags/4.48 /seaweedfs.tar.gz

# Rebuild the same upstream release with patched Go and HTTP/2 dependencies.
FROM --platform=$BUILDPLATFORM golang:1.26.9-alpine3.23@sha256:6750308561953d451210ed1a8086d36c8c8bdcbf672d38d9d0d424b8a7a0216c AS build
ENV GOTOOLCHAIN=local
COPY --from=seaweedfs-source /seaweedfs.tar.gz /tmp/seaweedfs.tar.gz
RUN echo "93199495e93a981451d1fefecb5aa71e12aa1692093eed2338bbfe7f08960bfb  /tmp/seaweedfs.tar.gz" | sha256sum -c - \
    && mkdir /src && tar -xzf /tmp/seaweedfs.tar.gz -C /src --strip-components=1 \
    && rm /tmp/seaweedfs.tar.gz
WORKDIR /src
ARG TARGETARCH
RUN --mount=type=cache,id=meetspace-seaweedfs-modules,target=/go/pkg/mod \
    --mount=type=cache,id=meetspace-seaweedfs-build,target=/root/.cache/go-build \
    go get golang.org/x/net@v0.60.0 \
    && CGO_ENABLED=0 GOOS=linux GOARCH=$TARGETARCH go build -trimpath \
        -ldflags "-s -w -X github.com/seaweedfs/seaweedfs/weed/util/version.COMMIT=530be3e37" \
        -o /out/weed ./weed \
    && go mod verify

# Preserve upstream runtime assets while excluding the vulnerable old binary
# from every layer of the final image, not just hiding it behind a new layer.
FROM chrislusf/seaweedfs:4.48@sha256:4e61d15fd35994cb1e43e1e553dff106794841fd9a99ade2fc8c8bfce4d7872d AS runtime-base
RUN apk upgrade --no-cache && rm /usr/bin/weed

FROM scratch AS runtime
COPY --from=runtime-base / /
COPY --from=build /out/weed /usr/bin/weed
COPY --from=build /src/LICENSE /usr/share/licenses/seaweedfs/LICENSE
ENV PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
WORKDIR /data
VOLUME /data
COPY dockers/object-storage/entrypoint.sh /usr/local/bin/recorder-object-storage
RUN chmod 755 /usr/local/bin/recorder-object-storage && chown -R 1000:1000 /data
USER 1000:1000
ENTRYPOINT ["/usr/local/bin/recorder-object-storage"]
