# YoMo

- build

  ```
  cargo build --release
  ```

- use Ollama as the LLM provider:

  ```
  ollama pull qwen3.5
  ```

- run YoMo server:

  ```
  ./target/release/yomo serve
  ```

- run YoMo serverless tool:

  ```
  ./target/release/yomo run --name get-weather ./demo/go/get_weather
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
