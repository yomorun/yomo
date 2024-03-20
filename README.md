<p align="center">
  <img width="200px" height="200px" src="https://blog.yomo.run/static/images/logo.png" />
</p>

# YoMo ![Go](https://github.com/yomorun/yomo/workflows/Go/badge.svg) [![codecov](https://codecov.io/gh/yomorun/yomo/branch/master/graph/badge.svg?token=MHCE5TZWKM)](https://codecov.io/gh/yomorun/yomo) [![Discord](https://img.shields.io/discord/770589787404369930.svg?label=discord&logo=discord&logoColor=ffffff&color=7389D8&labelColor=6A7EC2)](https://discord.gg/RMtNhx7vds)

YoMo is an open-source LLM Function Calling Framework for building Geo-distributed AI applications.
Built atop QUIC Transport Protocol and Stateful Serverless architecture, makes your AI application 
low-latency, reliable, secure, and easy.

💚 We care about: **Customer Experience in the Age of AI**

## 🌶 Features

|    | **Features**                                                                                                 |
| -- | ------------------------------------------------------------------------------------------------------------ |
| ⚡️ | **Low-latency** Guaranteed by implementing atop QUIC [QUIC](https://datatracker.ietf.org/wg/quic/documents/) |
| 🔐  | **Security** TLS v1.3 on every data packet by design                                                       |
| 📸  | **Stateful Serverless** Make your GPU serverless 10x faster                    |
| 🌎  | **Geo-Distributed Architecture** Brings AI inference closer to end users           |
| 🚀  | **Y3** a [faster than real-time codec](https://github.com/yomorun/y3-codec-golang)                           |

## 🚀 Getting Started

Let's implement a function calling with `sfn-currency-converter`:

### Step 1. Install CLI

```bash
curl -fsSL https://get.yomo.run | sh
```

Verify if the CLI was installed successfully

```bash
yomo version
```

### Step 2. Start the server

Prepare the configuration as `my-agent.yaml`

```yaml
name: ai-zipper
host: 0.0.0.0
port: 9000

auth:
  type: token
  token: SECRET_TOKEN

bridge:
  ai:
    server:
      addr: 0.0.0.0:8000 ## Restful API endpoint
      provider: openai ## LLM API Service we will use

    providers:
      azopenai:
        api_endpoint: https://<RESOURCE>.openai.azure.com
        deployment_id: <DEPLOYMENT_ID>
        api_key: <API_KEY>
        api_version: <API_VERSION>

      openai:
        api_key: sk-xxxxxxxxxxxxxxxxxxxxxxxxxxx
        model: gpt-4-1106-preview

      gemini:
        api_key: <GEMINI_API_KEY>

      cloudflare_azure:
        endpoint: https://gateway.ai.cloudflare.com/v1/<CF_GATEWAY_ID>/<CF_GATEWAY_NAME>
        api_key: <AZURE_API_KEY>
        resource: <AZURE_OPENAI_RESOURCE>
        deployment_id: <AZURE_OPENAI_DEPLOYMENT_ID>
        api_version: 2023-12-01-preview
```

Start the server:

```sh
YOMO_LOG_LEVEL=debug yomo serve -c my-agent.yaml
```

### Step 3. Write the function

First, let's define what this function do and how's the parameters required, these will be combined to prompt when invoking LLM.

```golang
type Parameter struct {
	Domain string `json:"domain" jsonschema:"description=Domain of the website,example=example.com"`
}

func Description() string {
	return `if user asks ip or network latency of a domain, you should return the result of the giving domain. try your best to dissect user expressions to infer the right domain names`
}

func InputSchema() any {
	return &Parameter{}
}
```

Create a Stateful Serverless Function to get the IP and Latency of a domain:

```golang
func handler(ctx serverless.Context) {
	fc, _ := ai.ParseFunctionCallContext(ctx)

	var msg Parameter
	fc.UnmarshalArguments(&msg)

// get ip of the domain
	ips, _ := net.LookupIP(msg.Domain)

	// get ip[0] ping latency
	pinger, _ := ping.NewPinger(ips[0].String())

	pinger.Count = 3
	pinger.Run()
	stats := pinger.Statistics()

	val := fmt.Sprintf("domain %s has ip %s with average latency %s", msg.Domain, ips[0], stats.AvgRtt)
	fc.Write(val)
}

```

Finally, let's run it

```bash
$ go run main.go

time=2024-03-19T21:43:30.583+08:00 level=INFO msg="connected to zipper" component=StreamFunction sfn_id=B0ttNSEKLSgMjXidB11K1 sfn_name=fn-get-ip-from-domain zipper_addr=localhost:9000
time=2024-03-19T21:43:30.584+08:00 level=INFO msg="register ai function success" component=StreamFunction sfn_id=B0ttNSEKLSgMjXidB11K1 sfn_name=fn-get-ip-from-domain zipper_addr=localhost:9000 name=fn-get-ip-from-domain tag=16
```

### Done, let's have a try

```sh
$ curl -i -X POST -H "Content-Type: application/json" -d '{"prompt":"compare nike and puma website speed"}' http://127.0.0.1:8000/invoke
HTTP/1.1 200 OK
Content-Length: 944
Connection: keep-alive
Content-Type: application/json
Date: Tue, 19 Mar 2024 13:30:14 GMT
Keep-Alive: timeout=4
Proxy-Connection: keep-alive

{
  "Content": "Based on the data provided for the domains nike.com and puma.com which include IP addresses and average latencies, we can infer the following about their website speeds:
  - Nike.com has an IP address of 13.225.183.84 with an average latency of 65.568333 milliseconds.
  - Puma.com has an IP address of 151.101.194.132 with an average latency of 54.563666 milliseconds.
  
  Comparing these latencies, Puma.com is faster than Nike.com as it has a lower average latency. 
  
  Please be aware, however, that website speed can be influenced by many factors beyond latency, such as server processing time, content size, and delivery networks among others. To get a more comprehensive understanding of website speed, you would need to consider additional metrics and possibly conductreal-time speed tests.",
  "FinishReason": "stop"
}
```

### Full Example Code

[Full LLM Function Calling Codes](./example/10-ai/)

## 📚 Documentation

Read more about YoMo at [yomo.run/docs](https://yomo.run/docs).

[YoMo](https://yomo.run) ❤️
[Vercel](https://vercel.com/?utm_source=yomorun&utm_campaign=oss), our
documentation website is

[![Vercel Logo](https://yomo.run/vercel.svg)](https://vercel.com/?utm_source=yomorun&utm_campaign=oss)

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

## License

[Apache License 2.0](http://www.apache.org/licenses/LICENSE-2.0.html)
