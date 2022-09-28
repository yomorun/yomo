# Rust wasm serverless function

## Add wasm32-wasi target

```sh
rustup target add wasm32-wasi
```

## Build

```sh
cargo build --release --target wasm32-wasi

cp target/wasm32-wasi/release/sfn.wasm ..
```
