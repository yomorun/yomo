#!/bin/bash

set -e

echo "Generating CA key and certificate:"
openssl req -x509 -sha256 -nodes -days 3650 -newkey rsa:2048 \
  -keyout ca.key -out ca.crt \
  -subj "/O=yomo Certificate Authority/"

echo "Generating CSR"
openssl req -out cert.csr -new -newkey rsa:2048 -nodes -keyout priv.key \
  -subj "/O=yomo/"

echo "Sign certificate:"
openssl x509 -req -sha256 -days 3650 -in cert.csr  -out cert.crt \
  -CA ca.crt -CAkey ca.key -CAcreateserial \
  -extfile <(printf "subjectAltName=DNS:localhost")

# debug output the certificate
openssl x509 -noout -text -in cert.crt

# we don't need the CA key, the serial number and the CSR any more
rm ca.key cert.csr


