# YoMo

- build

  ```
  cargo build --release

  ./target/release/yomo --help
  ```

- use Ollama as the LLM provider:

  ```
  ollama pull qwen3.5
  ```

- run YoMo server:

  ```
  RUST_LOG=debug ./target/release/yomo serve
  ```

- initialize a Node tool project:

  ```
  ./target/release/yomo init
  ```

- edit tool source:

  ```
  vi ./app/src/app.ts
  # or: vi ./app/app.go (when initialized with --language go)
  ```

- run YoMo serverless tool:

  ```
  RUST_LOG=debug ./target/release/yomo run --name get-weather ./app
  ```

- send a request:

  ```
  curl \
    --request POST \
    --url http://127.0.0.1:9001/v1/chat/completions \
    --header 'Content-Type: application/json' \
    --data '{
      "model": "qwen3.5",
      "messages": [
          {
              "role": "user",
              "content": "How is the weather in Beijing?"
          }
        ]
      }'
  ```
