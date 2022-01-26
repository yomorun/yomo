#!/bin/bash

SERVER_NAME=$1
if [[ -z "$SERVER_NAME" ]]; then
    SERVER_NAME='yomo-app.dev'
fi

echo "server name is: $SERVER_NAME"

mkdir -p tls-files
cd tls-files

# CA key
openssl ecparam \
    -genkey \
    -name secp384r1 \
    -out ca.key

# CA cert
openssl req \
    -x509 \
    -new \
    -nodes \
    -key ca.key \
    -days 3650 \
    -subj '/O=YoMo/CN=YoMo Root CA' \
    -out ca.crt

# Server key
openssl ecparam \
    -genkey \
    -name secp384r1 \
    -out server.key

# Server cert
openssl req \
    -new \
    -key server.key \
    -subj '/O=YoMo/CN=YoMo Server' \
    -addext "subjectAltName=DNS:localhost,DNS:$SERVER_NAME" | \
    openssl x509 \
        -req \
        -CA ca.crt \
        -CAkey ca.key \
        -CAserial ca.txt \
        -CAcreateserial \
        -days 3650 \
        -copy_extensions copy \
        -out server.crt

# Client key
openssl ecparam \
    -genkey \
    -name secp384r1 \
    -out client.key

# Client cert
openssl req \
    -new \
    -key client.key \
    -subj "/O=YoMo/CN=YoMo Client" | \
    openssl x509 \
        -req \
        -CA ca.crt \
        -CAkey ca.key \
        -CAserial ca.txt \
        -CAcreateserial \
        -days 3650 \
        -out client.crt
