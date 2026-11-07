#!/bin/sh
set -eu

CERT_DIR="${CERT_DIR:-dockers/https/certs}"
CERT_IP="${HTTPS_CERT_IP:-192.168.90.201}"
CERT_DNS="${HTTPS_CERT_DNS:-localhost}"
CA_NAME="${HTTPS_CA_NAME:-go-recorder-local-ca}"

mkdir -p "$CERT_DIR"

openssl genrsa -out "$CERT_DIR/$CA_NAME.key" 4096
cat > "$CERT_DIR/$CA_NAME.cnf" <<'EOF'
[req]
prompt = no
distinguished_name = ca_name
x509_extensions = ca_extensions

[ca_name]
CN = go-recorder local CA

[ca_extensions]
basicConstraints = critical,CA:TRUE
keyUsage = critical,keyCertSign,cRLSign
subjectKeyIdentifier = hash
authorityKeyIdentifier = keyid:always
EOF
openssl req -x509 -new -nodes \
  -key "$CERT_DIR/$CA_NAME.key" \
  -sha256 \
  -days 3650 \
  -out "$CERT_DIR/$CA_NAME.pem" \
  -config "$CERT_DIR/$CA_NAME.cnf" \
  -extensions ca_extensions

openssl genrsa -out "$CERT_DIR/go-recorder-server.key" 2048
openssl req -new \
  -key "$CERT_DIR/go-recorder-server.key" \
  -out "$CERT_DIR/go-recorder-server.csr" \
  -subj "/CN=$CERT_IP"

cat > "$CERT_DIR/go-recorder-server.ext" <<EOF
authorityKeyIdentifier=keyid,issuer
subjectKeyIdentifier=hash
basicConstraints=critical,CA:FALSE
keyUsage=critical,digitalSignature,keyEncipherment
extendedKeyUsage=serverAuth
subjectAltName=@alt_names

[alt_names]
IP.1=$CERT_IP
IP.2=127.0.0.1
DNS.1=$CERT_DNS
DNS.2=localhost
EOF

openssl x509 -req \
  -in "$CERT_DIR/go-recorder-server.csr" \
  -CA "$CERT_DIR/$CA_NAME.pem" \
  -CAkey "$CERT_DIR/$CA_NAME.key" \
  -CAcreateserial \
  -out "$CERT_DIR/go-recorder-server.crt" \
  -days 825 \
  -sha256 \
  -extfile "$CERT_DIR/go-recorder-server.ext"

chmod 600 "$CERT_DIR"/*.key
chmod 644 "$CERT_DIR"/*.pem "$CERT_DIR"/*.crt

echo "Generated HTTPS certs in $CERT_DIR"
echo "Trust CA file on client: $CERT_DIR/$CA_NAME.pem"
