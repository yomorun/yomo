# Basic TLS example

TLS is supported by YoMo. In order to run YoMo services with TLS encryption, it's not necessary to change any code or recompile the program; instead, the only thing you need to do is to add 4 environment variables when starting the service:

- `YOMO_ENV` ( = production )
- `YOMO_TLS_CACERT_FILE`
- `YOMO_TLS_CERT_FILE`
- `YOMO_TLS_KEY_FILE`

This example will show you how to build up a YoMo service with self-signed certificates for production environments.

## 1. Generate self-signed certificates

To Run this SHELL script you'll need OpenSSL>=1.1.1 .

```bash
./generate_ca.sh

./generate_server.sh

./generate_client.sh source

./generate_client.sh flow
```

By default the YoMo server name is `yomo-app.dev`. It's possible to create the server certificate for your own DNS name, e.g. `abc.test`:

```bash
./generate_server.sh abc.test
```

If successful, 8 files should be generated in the `tls-files` folder.

- ca.crt
- ca.key
- server.crt
- server.key
- client_source.crt
- client_source.key
- client_flow.crt
- client_flow.key


## 2. Add yomo-app.dev to hostname file

```bash
echo '127.0.0.1 yomo-app.dev' | sudo tee -a /etc/hosts
```

## 3. Run YoMo Zipper (Server) with TLS encryption

```bash
YOMO_ENV=production \
YOMO_TLS_CACERT_FILE=tls-files/ca.crt \
YOMO_TLS_CERT_FILE=tls-files/server.crt \
YOMO_TLS_KEY_FILE=tls-files/server.key \
yomo serve -c ../basic/zipper/workflow.yaml
```

## 4. Run YoMo Stream Function (Client) with TLS encryption

```bash
YOMO_ENV=production \
YOMO_TLS_CACERT_FILE=tls-files/ca.crt \
YOMO_TLS_CERT_FILE=tls-files/client_flow.crt \
YOMO_TLS_KEY_FILE=tls-files/client_flow.key \
YOMO_ADDR=yomo-app.dev:9000 \
go run ../basic/flow/main.go
```

## 5. Run YoMo Source (Client) with TLS encryption

```bash
YOMO_ENV=production \
YOMO_TLS_CACERT_FILE=tls-files/ca.crt \
YOMO_TLS_CERT_FILE=tls-files/client_source.crt \
YOMO_TLS_KEY_FILE=tls-files/client_source.key \
YOMO_ADDR=yomo-app.dev:9000 \
go run ../basic/source/main.go
```
