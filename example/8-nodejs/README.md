# Implementing YoMo Stream Function using Deno

Nowadays, TypeScript is one of the most popular programming
languages. It's easy to learn, yet still powerful, thus suitable for serverless
mode. YoMo has integrated the Nodejs runtime for developers to implement TS
serverless functions.

## Install Nodejs runtime

https://nodejs.org/en/download/package-manager

## Run the demo example

- Write your own serverless function by reference to [app.ts](app.ts)

- Start YoMo zipper

  ```sh
  yomo serve -c ../10-ai/zipper.yaml
  ```

- Start sfn

  ```sh
  yomo run app.ts
  ```

- Invoke the LLM Function

```sh
$ curl -i http://127.0.0.1:8000/v1/chat/completions -H "Content-Type: application/json" -d '{
  "messages": [
    {
      "role": "system",
      "content": "You are a test assistant."
    },
    {
      "role": "user",
      "content": "How is the weather in Korea and UK today?"
    }
  ],
  "stream": false
}'
HTTP/1.1 200 OK
Content-Type: application/json
Date: Mon, 04 Nov 2024 05:10:24 GMT
Content-Length: 824

{"id":"chatcmpl-APje3REU3DvpZ9yANpLIpLf6uyWNk","object":"chat.completion","created":1730697023,"model":"gpt-4o-2024-08-06","choices":[{"index":0,"message":{"role":"assistant","content":"Today, the weather in Seoul, Korea is quite warm with a temperature of 33°C. In London, UK, it's cooler with a temperature of 19°C."},"finish_reason":"stop","content_filter_results":{"hate":{"filtered":false},"self_harm":{"filtered":false},"sexual":{"filtered":false},"violence":{"filtered":false},"jailbreak":{"filtered":false,"detected":false},"profanity":{"filtered":false,"detected":false}}}],"usage":{"prompt_tokens":174,"completion_tokens":81,"total_tokens":187,"prompt_tokens_details":{"audio_tokens":0,"cached_tokens":0},"completion_tokens_details":{"audio_tokens":0,"reasoning_tokens":0}},"system_fingerprint":"fp_159d8341cc"}
```