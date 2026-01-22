# YoMo

v2 version is fully rewritten in Rust.

- build:

```
cargo build
```

- run server:

```
RUST_LOG=info ./target/debug/yomo serve
```

- run sfn:

```
RUST_LOG=info ./target/debug/yomo run --name uppercase ./serverless/go/uppercase
```

- send request:

```
curl -d '{"args": "Hello, YoMo!"}' \
  -H 'Content-type: application/json' \
  http://127.0.0.1:9001/sfn/uppercase
```

- send stream request:

```
curl -d '{"args": "Welcome to build stream serverless functions."}' \
  -H 'Content-type: application/json' \
  http://127.0.0.1:9001/sfn/uppercase/sse
```
