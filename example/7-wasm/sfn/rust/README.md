# Rust wasm serverless function

## Development

Notice that we have provided a Rust [crate](https://crates.io/crates/yomo),
hence it will be convenient for developers by importing this crate to your app
istead of implementing our wasm api spec. See [src/lib.rs](src/lib.rs) for more
details.

## Add wasm32-wasip1 target

```sh
rustup target add wasm32-wasip1
```

## Build

```sh
cargo build --release --target wasm32-wasip1

cp target/wasm32-wasi/release/sfn.wasm ..
```
