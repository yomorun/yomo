#!/bin/bash

mkdir -p tls
cd tls

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
