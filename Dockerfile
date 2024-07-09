FROM golang:1.21-alpine AS builder

RUN apk update && apk add --no-cache make && apk --no-cache add git

WORKDIR /

COPY . .

RUN GO_MODULE=$(go list -m) \
    && TAG=$(git describe --tags 2>/dev/null || git rev-parse --short HEAD) \
    && go build -o bin/yomo -trimpath -ldflags "-s -w -X ${GO_MODULE}/cli.Version=${TAG}" ./cmd/yomo/main.go

FROM alpine:3.17

RUN apk --no-cache add ca-certificates

WORKDIR /usr/local/bin

COPY --from=builder /bin/yomo .

CMD ["./yomo"]