<p align="center">
  <img width="200px" height="200px" src="https://blog.yomo.run/static/images/logo.png" />
</p>

# YoMo ![Go](https://github.com/yomorun/yomo/workflows/Go/badge.svg) [![codecov](https://codecov.io/gh/yomorun/yomo/branch/master/graph/badge.svg?token=MHCE5TZWKM)](https://codecov.io/gh/yomorun/yomo) [![Discord](https://img.shields.io/discord/770589787404369930.svg?label=discord&logo=discord&logoColor=ffffff&color=7389D8&labelColor=6A7EC2)](https://discord.gg/RMtNhx7vds)

YoMo is an open-source LLM Function Calling Framework for building scalable and ultra-fast AI Agents.
💚 We care about: **Empowering Exceptional Customer Experiences in the Age of AI**

We believe that seamless and responsive AI interactions are key to delivering outstanding customer experiences. YoMo is built with this principle at its core, focusing on speed, reliability, and scalability.


## 🌶 Features

|    | **Features** |    |
| -- | ------------ | -- |
| ⚡️ | **Low-Latency MCP** | Guaranteed by implementing atop the [QUIC Protocol](https://datatracker.ietf.org/wg/quic/documents/). Experience significantly faster communication between AI agents and MCP server. |
| 🔐  | **Enhanced Security** | TLS v1.3 encryption is applied to every data packet by design, ensuring robust security for your AI agent communications. |
| 🚀  | **Strongly-Typed Language** | Build robust AI agents with complete confidence through type-safe function calling, enhanced error detection, and seamless integration capabilities. Type safety prevents runtime errors, simplifies testing, and enables IDE auto-completion. Currently support TypeScript and Go. |
| 📸  | **Effortless Serverless DevOps** | Streamline the entire lifecycle of your LLM tools, from development to deployment. Significantly reduces operational overhead, allowing you to focus exclusively on creating innovative AI agent functionalities. |
| 🌎  | **Geo-Distributed Architecture** | Bring AI inference and tools closer to your users with our globally distributed architecture, resulting in significantly faster response times and a superior user experience for your AI agents. |

## 🚀 Getting Started

Let's build a simple AI agent with LLM Function Calling to provide weather information:

### Step 1. Install CLI

```bash
curl -fsSL https://get.yomo.run | sh
```

Verify the installation:

```bash
yomo version
```

### Step 2. Start the server

Create a configuration file `my-agent.yaml`:

```yaml
name: my-agent
host: 0.0.0.0
port: 9000

auth:
  type: token
  token: SECRET_TOKEN

bridge:
  ai:
    server:
      addr: 0.0.0.0:9000 ## OpenAI API compatible endpoint
      provider: vllm     ## llm to use

    providers:
      vllm:
        api_endpoint: http://127.0.0.1:8000/v1
        model: meta-llama/Llama-4-Scout-17B-16E-Instruct

      ollama:
        api_endpoint: http://localhost:11434
```

Launch the server:

```sh
yomo serve -c my-agent.yaml
```

### Step 3. Implement the LLM Function Calling

Create a type-safe function that retrieves weather data:

```typescript
export const description = 'Get the current weather for `city`'

export type Argument = {
  /**
   * The name of the city to be queried
   */
  city: string;
}

export async function handler(args: Argument) {
  // Simulate a weather API call
  let temperature = Math.floor(Math.random() * 41)
  // Return the result to LLM
  return { 
    city: args.city, 
    temperature: temperature,
    feels_like: 11.9,
    rain: false,
  }
}
```

Finished, now, let's run it:

```bash
$ yomo run -n get-weather
```

### Done, let's have a try

```sh
$ curl http://127.0.0.1:9000/v1/chat/completions \
-H "Content-Type: application/json" \
-H "Authorization: Bearer SECRET_TOKEN" \
-d '{
  "messages": [
    {
      "role": "user",
      "content": "I am going for a hike on the Yarra Bend Park Loop. What should I wear?"
    }
  ],
  "stream": false
}'
```

You'll receive a helpful response like this:

```
For your hike on the Yarra Bend Park Loop, the current weather is clear with a temperature of approximately 12.3°C (feels like 11.9°C). 

Here are some suggestions on what to wear: 

1. **Layers**: Start with a base layer such as a moisture-wicking t-shirt. Add a light sweater or fleece for warmth since it can be chilly. 
2. **Jacket**: Bring a lightweight jacket or windbreaker to keep warm, especially as it is breezy with a southeast wind at 6 km/h with gusts up to 14 km/h. 
3. **Pants**: Comfortable hiking pants or leggings will be suitable. 
4. **Footwear**: Wear sturdy hiking boots or shoes with good grip. 
5. **Accessories**: Consider a hat or beanie for warmth, and bring gloves if you tend to get cold easily. 
6. **Backpack**: Carry a small backpack with water, snacks, and any additional layers you might need. 

Since there is **no rain** expected, you shouldn't need waterproof gear, but it's always wise to check the latest forecast before heading out. Enjoy your hike!
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


## ❓ FAQ

### Getting Started

**Q: What is YoMo?**  
A: YoMo is an open-source LLM Function Calling Framework for building scalable and ultra-fast AI Agents. Built atop QUIC Protocol for low-latency MCP communication, with TLS v1.3 encryption by design.

**Q: How is YoMo different from LangChain?**  
A: LangChain focuses on Python orchestration; YoMo focuses on **geo-distributed AI inference infrastructure** with QUIC-based low-latency communication. YoMo brings AI inference closer to users globally.

**Q: What languages are supported?**  
A: TypeScript and Go. Both provide type-safe function calling with IDE auto-completion.

**Q: How do I install YoMo?**  
A:
```bash
curl -fsSL https://get.yomo.run | sh
yomo version
```

### Core Features

**Q: What is Low-Latency MCP?**  
A: YoMo implements MCP atop QUIC Protocol, providing significantly faster communication between AI agents and MCP servers compared to HTTP-based approaches.

**Q: How does Geo-Distributed Architecture work?**  
A: YoMo enables deploying AI inference close to end users globally. Instead of central data centers, applications run near users for faster response times.

**Q: What is Serverless DevOps?**  
A: YoMo streamlines LLM tool lifecycle from development to deployment, reducing operational overhead for AI agent functionalities.

### Configuration

**Q: How do I configure the server?**  
A: Create a YAML config file (`my-agent.yaml`) with name/host/port/auth/bridge settings:
```yaml
name: my-agent
host: 0.0.0.0
port: 9000
auth:
  type: token
  token: SECRET_TOKEN
bridge:
  ai:
    server:
      addr: 0.0.0.0:9000
      provider: vllm
```

**Q: Which LLM providers are supported?**  
A: Configure `vllm` or `ollama` providers. YoMo provides OpenAI API compatible endpoint at `http://localhost:9000/v1`.

**Q: How do I use Ollama?**  
A: Configure in YAML:
```yaml
bridge:
  ai:
    providers:
      ollama:
        api_endpoint: http://localhost:11434
```
Ensure Ollama is running (`ollama serve`).

### Function Calling

**Q: How do I implement a function?**  
A: Create TypeScript file with `description`, `Argument` type, and `handler` function:
```typescript
export const description = 'Get weather for city'
export type Argument = { city: string }
export async function handler(args: Argument) {
  return { city: args.city, temperature: 20 }
}
```

**Q: How do I run a function?**  
A:
```bash
yomo run -n get-weather
```

**Q: How do I call the function from LLM?**  
A: Send chat completion request to YoMo's OpenAI-compatible endpoint:
```bash
curl http://localhost:9000/v1/chat/completions \
  -H "Authorization: Bearer SECRET_TOKEN" \
  -d '{"messages":[{"role":"user","content":"What should I wear?"}]}'
```

### Troubleshooting

**Q: Server fails to start?**  
A: Check YAML configuration file syntax. Ensure `auth.token` is set and `bridge.ai.server.addr` matches host/port.

**Q: Function calling fails?**  
A: Ensure function file has correct `description`, `Argument` type with JSDoc comments, and `handler` returns proper response.

**Q: Ollama connection fails?**  
A: Verify Ollama is running (`ollama serve`) and `api_endpoint` is correct in YAML config.

**Q: Authentication fails?**  
A: Check `Authorization: Bearer SECRET_TOKEN` header matches token in YAML config.

### More Help

Check [Serverless LLM Function Calling Examples](https://github.com/yomorun/llm-function-calling-examples) or visit [yomo.run](https://yomo.run/) for documentation.

---

## 🦸 Contributing

First off, thank you for considering making contributions. It's people like you that make YoMo better. There are many ways in which you can participate in the project, for example:

- File a [bug report](https://github.com/yomorun/yomo/issues/new?assignees=&labels=bug&template=bug_report.md&title=%5BBUG%5D). Be sure to include information like what version of YoMo you are using, what your operating system is, and steps to recreate the bug.
- Suggest a new feature.
- Read our [contributing guidelines](https://github.com/yomorun/yomo/blob/master/CONTRIBUTING.md) to learn about what types of contributions we are looking for.
- We have also adopted a [code of conduct](https://github.com/yomorun/yomo/blob/master/CODE_OF_CONDUCT.md) that we expect project participants to adhere to.

## License

[Apache License 2.0](http://www.apache.org/licenses/LICENSE-2.0.html)
