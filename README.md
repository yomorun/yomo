# YoMo

v2 version is fully rewritten in Rust.

- build:

```
cargo build
```

- run server:

```
RUST_LOG=debug ./target/debug/yomo serve
```

- run sfn:

```
RUST_LOG=debug ./target/debug/yomo run --name uppercase
```

- send request:

```
curl -d '{"args":"Hello, YoMo!"}' \
  -H 'Content-type: application/json' \
  http://127.0.0.1:9001/sfn/uppercase
```
