# Go wasm serverless function

## Install TinyGo

The official Go compiler hasn't support WASI yet, therefore we need to use
[TinyGo](https://tinygo.org) instead.

https://tinygo.org/getting-started/install/

## Build

```sh
tinygo build -o sfn.wasm -no-debug -target wasi

cp sfn.wasm ../
```
