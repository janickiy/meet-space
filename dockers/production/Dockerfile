FROM --platform=$BUILDPLATFORM node:24-alpine AS admin-build
ARG NODE_OPTIONS
WORKDIR /admin
COPY admin/package.json admin/package-lock.json ./
RUN npm ci
COPY admin/ ./
ARG BUILD_VERSION=dev
ENV VITE_BUILD_VERSION=$BUILD_VERSION
RUN npm run build

FROM nginx:1.30.6-alpine@sha256:47928bf67eb62a1863cb08d9ecb52d3e704304ba614eaee17884204677702951 AS runtime
# Nginx остаётся в поддерживаемой ветке; обновляются уязвимые expat/pcre2.
# Keep the proxy on the same fixed curl floor as the frontend runtime image.
RUN apk upgrade --no-cache \
    && apk add --no-cache 'curl>=8.23.0-r0' 'libcurl>=8.23.0-r0'
COPY --from=admin-build /admin/dist /usr/share/nginx/admin
COPY dockers/production/admin.conf.template /etc/meetspace/admin.conf.template
COPY dockers/production/25-admin-config.sh /docker-entrypoint.d/25-admin-config.sh
RUN chmod +x /docker-entrypoint.d/25-admin-config.sh
ENV ADMIN_ENABLED=false
EXPOSE 443 8081
