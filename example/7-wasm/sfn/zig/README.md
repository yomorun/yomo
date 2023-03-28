# Zig wasm serverless function

## Build

```sh
zig build-lib src/main.zig -target wasm32-wasi -dynamic --name sfn
cp sfn.wasm ../
```
