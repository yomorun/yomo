# Implementing YoMo Stream Function using Deno

Nowadays, JavaScript (or TypeScript) is one of the most popular programming
languages. It's easy to learn, yet still powerful, thus suitable for serverless
mode. YoMo has integrated the Deno runtime for developers to implement JS/TS
serverless functions.

## Install Deno runtime

https://deno.land/#installation

## Run the demo example

- Write your own serverless function by reference to [app.ts](app.ts)

- Start YoMo zipper

  ```sh
  yomo serve -c ../uppercase/workflow.yaml
  ```

- Start the JS/TS serverless function

  ```sh
  yomo run app.ts
  ```

- Start Source & Sink

  ```sh
  cd ../uppercase/source

  go run main.go
  ```
