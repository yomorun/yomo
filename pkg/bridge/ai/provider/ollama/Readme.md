# Ollama LLM inference provider

Build up your YoMo AI architecture with fully open-source dependencies.

## 1. Install Ollama

Follow the Ollama doc:

<https://github.com/ollama/ollama?tab=readme-ov-file#ollama>

## 2. Run the Mistral model

Notice that only the Mistral v0.3+ models are supported currently.

```sh
ollama run mistral:7b
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
        api_endpoint: "http://localhost:11434/"
```

```sh
yomo serve -c config.yml
```

## 4. Start YoMo serverless function

[example](../../../../../example/10-ai/README.md)
