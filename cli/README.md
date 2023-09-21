# YoMo CLI

Command-line tools for YoMo

## Binary

`curl -fsSL https://get.yomo.run | sh`

## Build from source

[Installing Go](https://golang.org/doc/install)

You can easily install the latest release globally by running:

```sh
go install github.com/yomorun/yomo/cmd/yomo@latest
```

Or you can install into another directory:

```sh
env GOBIN=/bin go install github.com/yomorun/yomo/cmd/yomo@latest
```

## Getting Started

### 1. YoMo-Zipper

#### Configure YoMo-Zipper `config.yaml`

See [../example/config.yaml](../example/config.yaml)

#### Run

```sh
yomo serve --config ../example/config.yaml
```

### 2. Source

#### Write a source app

See [../example/9-cli/source/main.go](../example/9-cli/source/main.go)

#### Run

```sh
cd ../example/9-cli/source

go run main.go
```

### 3. Stream Function

#### Write the serverless function

See [../example/9-cli/sfn/main.go](../example/9-cli/sfn/main.go)

#### Build

Build the app.go to a WebAssembly file.

```sh
cd ../example/9-cli/sfn

yomo build
```

#### Run

```sh
yomo run sfn.wasm
```
