<p align="center">
  <img width="200px" height="200px" src="https://blog.yomo.run/static/images/logo.png" />
</p>

# YoMo [![codecov](https://codecov.io/gh/yomorun/yomo/branch/main/graph/badge.svg)](https://codecov.io/gh/yomorun/yomo)

YoMo is an open-source LLM Function Calling Framework for building scalable and ultra-fast AI Agents.
💚 We care about: **Empowering Exceptional Customer Experiences in the Age of AI**

We believe that seamless and responsive AI interactions are key to delivering outstanding customer experiences. YoMo is built with this principle at its core, focusing on speed, reliability, and scalability.

## 🌶 Features

|    | **Features** |    |
| -- | ------------ | -- |
| ⚡️ | **Serverless LLM Tools** | Deploy and Manage LLM Tools / Skills seamlessly. |
| 🔐 | **Enhanced Security** | TLS v1.3 encryption is applied to every data packet by design, ensuring robust security for your AI agent communications. |
| 📸 | **Effortless Agents DevOps** | Streamline the entire lifecycle of your LLM tools, from development to deployment. Significantly reduces operational overhead, allowing you to focus exclusively on creating innovative AI agent functionalities. |
| 🌎 | **Geo-Distributed Architecture** | Bring AI inference and tools closer to your users with our globally distributed architecture, resulting in significantly faster response times and a superior user experience for your AI agents. |

## 🚀 Getting Started

Let's build a simple AI agent with LLM Function Calling to provide weather information:

### Step 1. Install CLI

```sh
curl -fsSL https://get.yomo.run | sh
```

Verify the installation:

```sh
yomo --version
```

### Step 2. Start the server

Use Ollama as the LLM provider:

```sh
ollama pull ornith
```

Launch the server:

```sh
yomo serve
```

You can also use the `--config` flag to specify a custom coniguration yaml file.

### Step 3. Implement the LLM Function Calling

```sh
yomo init
```

Finished, now, let's run it:

```bash
yomo run -n get-weather ./app
```

### Done, let's have a try

```sh
curl http://127.0.0.1:9001/v1/chat/completions \
-H "Content-Type: application/json" \
-d '{
  "messages": [
    {
      "role": "user",
      "content": "I am going for a hike on the Yarra Bend Park Loop. What should I wear?"
    }
  ]
}'
```

You'll receive a helpful response like this:

```
Yarra Bend Park is on the Yarra River and gets misty/foggy when it overflows from its channel into the park—this typically occurs on damp autumns around November. Today's conditions are mild, warm, dry, with no fog expected today but a chance of light rain or drizzle possible tomorrow afternoon.

**Clothing for your hike:**
- **Base/mid-layer:** A long-sleeve top is enough. If it gets chilly at the river bank (a 2°C drop is possible), add a fleece mid-layer rather than relying on just an autumn outer shell.
- **Pants:** Jeans are okay if you want them, but light hiking pants or athletic wear are more comfortable and dry faster.
- **Footwear:** You'll be on forest trails around the Yarra River—sturdy sneakers will do for the loop today. If there's rain tomorrow afternoon, bring a pair of boots.
- **Rain gear:** Carry an umbrella just in case you get caught on the trail after the rain passes this weekend.

**Don't worry about mosquitoes this month.** They arrive in March/April when it gets hot and dry—and that's right around the time summer solstice fog starts forming (June). In November, no mosquitoes at all.
```

### Explore More Examples

Check out our [Servereless LLM Function Calling Examples](https://github.com/yomorun/llm-function-calling-examples) for more use cases and inspiration.

## 📚 Documentation

Read more about YoMo on [yomo.run](https://yomo.run/).

## 🎯 Focuses on Geo-distributed AI Inference Infra

It’s no secret that today’s users want instant AI inference, every AI 
application is more powerful when it response quickly. But, currently, when we
talk about `distribution`, it represents **distribution in data center**. The AI model is
far away from their users from all over the world.

If an application can be deployed anywhere close to their end users, solve the
problem, this is **Geo-distributed System Architecture**:

<img width="580" alt="yomo geo-distributed system" src="https://user-images.githubusercontent.com/65603/162367572-5a0417fa-e2b2-4d35-8c92-2c95d461706d.png">

## 🦸 Contributing

First off, thank you for considering making contributions. It's people like you
that make YoMo better. There are many ways in which you can participate in the
project, for example:

- File a
  [bug report](https://github.com/yomorun/yomo/issues/new?assignees=&labels=bug&template=bug_report.md&title=%5BBUG%5D).
  Be sure to include information like what version of YoMo you are using, what
  your operating system is, and steps to recreate the bug.
- Suggest a new feature.
- Read our
  [contributing guidelines](https://github.com/yomorun/yomo/blob/master/CONTRIBUTING.md)
  to learn about what types of contributions we are looking for.
- We have also adopted a
  [code of conduct](https://github.com/yomorun/yomo/blob/master/CODE_OF_CONDUCT.md)
  that we expect project participants to adhere to.


## Devleopment

- Build

  ```sh
  cargo build --release

  ./target/release/yomo --help
  ```

- Use Ollama as the LLM provider:

  ```sh
  ollama pull ornith
  ```

- Run YoMo server:

  ```sh
  ./target/release/yomo serve
  ```

- Initialize a Serverless LLM Tool project:

  ```sh
  ./target/release/yomo init
  ```

  then edit `./app/src/app.ts` in the project.

- Run YoMo serverless tool:

  ```sh
  ./target/release/yomo run --name get-weather ./app
  ```

- Send a request to the LLM agent:

  ```sh
  curl \
    --request POST \
    --url http://127.0.0.1:9001/v1/chat/completions \
    --header 'Content-Type: application/json' \
    --data '{
      "messages": [
          {
              "role": "user",
              "content": "How is the weather in London?"
          }
        ]
      }'
  ```

- Send a request to the serverless function directly:

  ```sh
  curl \
    --request POST \
    --url http://127.0.0.1:9001/tool/get-weather \
    --header 'Content-Type: application/json' \
    --data '{
      "args":"{\"city\":\"London\"}"
    }'
  ```

  
## License

[Apache License 2.0](http://www.apache.org/licenses/LICENSE-2.0.html)
