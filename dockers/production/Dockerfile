FROM nginx:1.30.6-alpine@sha256:47928bf67eb62a1863cb08d9ecb52d3e704304ba614eaee17884204677702951 AS runtime
# Nginx остаётся в поддерживаемой ветке; обновляются уязвимые expat/pcre2.
# Keep the proxy on the same fixed curl floor as the frontend runtime image.
RUN apk upgrade --no-cache \
    && apk add --no-cache 'curl>=8.23.0-r0' 'libcurl>=8.23.0-r0'
