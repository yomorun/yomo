#!/bin/bash

CLIENT_NAME=$1
if [[ -z "$CLIENT_NAME" ]]; then
    CLIENT_NAME='0'
fi

echo "client name is: $CLIENT_NAME"

cd tls

# Client key
openssl ecparam \
    -genkey \
    -name secp384r1 \
    -out client_$CLIENT_NAME.key

# Client cert
openssl req \
    -new \
    -key client_$CLIENT_NAME.key \
    -subj "/O=YoMo/CN=YoMo Client" | \
    openssl x509 \
        -req \
        -CA ca.crt \
        -CAkey ca.key \
        -CAserial ca.txt \
        -CAcreateserial \
        -days 3650 \
        -out client_$CLIENT_NAME.crt
