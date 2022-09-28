# Zig wasm serverless function

## Build

```sh
zig build-exe src/main.zig -target wasm32-wasi --name sfn
cp sfn.wasm ../
```
