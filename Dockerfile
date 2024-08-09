FROM golang:1.21-alpine AS builder

RUN apk update && apk add --no-cache make && apk --no-cache add git

WORKDIR /

COPY . .

RUN GO_MODULE=$(go list -m) \
    && TAG=$(git describe --tags 2>/dev/null || git rev-parse --short HEAD) \
    && go build -o /bin/yomo -trimpath -ldflags "-s -w -X ${GO_MODULE}/cli.Version=${TAG}" ./cmd/yomo/main.go

FROM alpine:3.17

RUN apk --no-cache add ca-certificates

WORKDIR /zipper

COPY --from=builder /bin/yomo /usr/local/bin

RUN echo -e "name: zipper\nhost: 0.0.0.0\nport: 9000\nbridge:\n  ai:\n    server:\n      addr: 0.0.0.0:8000\n      provider: openai\n\n    providers:\n      openai:" > /zipper/config.yaml

EXPOSE 9000/udp
EXPOSE 8000/tcp

CMD ["/usr/local/bin/yomo", "serve", "-c", "config.yaml"]
