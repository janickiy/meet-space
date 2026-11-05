#!/bin/sh
# The disabled state has no Nginx listener. Recreating the proxy applies changes.
set -eu
destination=/etc/nginx/conf.d/meetspace-admin.conf
case "${ADMIN_ENABLED:-false}" in
    false) rm -f "$destination"; exit 0 ;;
    true) ;;
    *) printf '%s\n' 'ADMIN_ENABLED must be true or false' >&2; exit 1 ;;
esac
cert=${ADMIN_TLS_CERT:-/run/certs/fullchain.pem}
key=${ADMIN_TLS_KEY:-/run/certs/privkey.pem}
for path in "$cert" "$key"; do
    case "$path" in
        /*) ;;
        *) printf '%s\n' 'Admin TLS paths must be absolute' >&2; exit 1 ;;
    esac
    case "$path" in
        *[!a-zA-Z0-9_./-]*) printf '%s\n' 'Invalid admin TLS path' >&2; exit 1 ;;
    esac
    [ -r "$path" ] || { printf '%s\n' 'Admin TLS certificate or key is unavailable' >&2; exit 1; }
done
sed -e "s|@@ADMIN_TLS_CERT@@|$cert|g" -e "s|@@ADMIN_TLS_KEY@@|$key|g" \
    /etc/meetspace/admin.conf.template > "$destination"
