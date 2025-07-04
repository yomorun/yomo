# Ollama LLM Inference Provider

Build up your YoMo AI architecture with fully open-source dependencies.

## 1. Install Ollama

Follow the Ollama doc:

<https://github.com/ollama/ollama?tab=readme-ov-file#ollama>

## 2. Run the model

```sh
ollama run mistral
```

## 3. Start YoMo Zipper

By default, ollama server will be listening at the port 11434.

Then the config file could be:

```yml
name: Service
host: 0.0.0.0
port: 9000

bridge:
  ai:
    server:
      addr: "localhost:8000"
      provider: ollama
    providers:
      ollama:
        model: mistral # Make sure this model is available in Ollama, e.g. run 'ollama run mistral'
        api_endpoint: "http://localhost:11434/v1"
```

```sh
yomo serve -c config.yml
```

## 4. Start YoMo serverless function

[LLM Function Calling Examples](https://github.com/yomorun/llm-function-calling-examples)
