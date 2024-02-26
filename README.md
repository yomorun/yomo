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
      provider: azopenai ## LLM API Service we will use

    providers:
      azopenai:
        api_key: <YOUR_AZURE_OPENAI_API_KEY>
        api_endpoint: <YOUR_AZURE_OPENAI_ENDPOINT>

      openai:
        api_key: <OPENAI_API_KEY>
        model: <OPENAI_MODEL>

      gemini:
        api_key: <GEMINI_API_KEY>

      huggingface:
        model:
```

Start the server:

```sh
YOMO_LOG_LEVEL=debug yomo serve -c my-agent.yaml
```

### Step 3. Write the function

First, let's define what this function do and how's the parameters required, these will be combined to prompt when invoking LLM.

```golang
func Description() string {
	return "Get the current exchange rates"
}

type Parameter struct {
	SourceCurrency string  `json:"source" jsonschema:"description=The source currency to be queried in 3-letter ISO 4217 format"`
	TargetCurrency string  `json:"target" jsonschema:"description=The target currency to be queried in 3-letter ISO 4217 format"`
	Amount         float64 `json:"amount" jsonschema:"description=The amount of the USD currency to be converted to the target currency"`
}

func InputSchema() any {
	return &Parameter{}
}
```

Retrieve the real-time exchange rate by calling the openexchangerates.org API.:

```golang
type Rates struct {
	Rates map[string]float64 `json:"rates"`
}

func fetchRate(sourceCurrency string, targetCurrency string, amount float64) (float64, error) {
	resp, _ := http.Get(fmt.Sprintf("https://openexchangerates.org/api/latest.json?app_id=%s&base=%s&symbols=%s", os.Getenv("API_KEY"), sourceCurrency, targetCurrency))
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var rt *Rates
	_ = json.Unmarshal(body, &rt)

  return rates.Rates[targetCurrency], nil
}
```

Wrap to a Stateful Serverless Function:

```golang
func handler(ctx serverless.Context) {
	fcCtx, _ := ai.ParseFunctionCallContext(ctx)

	var msg Parameter
	fcCtx.UnmarshalArguments(&msg)

	rate, _ := fetchRate(msg.SourceCurrency, msg.TargetCurrency, msg.Amount)
  	result = fmt.Sprintf("%f", msg.Amount*rate)

	fcCtx.SetRetrievalResult(fmt.Sprintf("based on today's exchange rate: %f, %f %s is equivalent to approximately %f %s",rate, msg.Amount, msg.SourceCurrency, msg.Amount*rate, msg.TargetCurrency))
	fcCtx.Write(result)
}
```

Finally, let's run it

```bash
$ API_KEY=<get_from_openexchangerates.org> go run main.go

time=2024-02-26T17:29:52.868+08:00 level=INFO msg="connected to zipper" component=StreamFunction sfn_id=GqfKopi2ECx7GIlzw6ZL3 sfn_name=fn-exchange-rates zipper_addr=localhost:9000
time=2024-02-26T17:29:52.869+08:00 level=INFO msg="register ai function success" component=StreamFunction sfn_id=GqfKopi2ECx7GIlzw6ZL3 sfn_name=fn-exchange-rates zipper_addr=localhost:9000 name=fn-exchange-rates tag=16
```

### Done, let's have a try

```sh
$ curl -i -X POST -H "Content-Type: application/json" -d '{"prompt":"How much is 100 dollar in Korea and UK currency"}' http://127.0.0.1:8000/invoke

HTTP/1.1 200 OK
Transfer-Encoding: chunked
Connection: keep-alive
Content-Type: text/event-stream
Date: Mon, 26 Feb 2024 09:30:35 GMT
Keep-Alive: timeout=4
Proxy-Connection: keep-alive

event:result
data: {"req_id":"7YU0SY","result":"78.920600","retrieval_result":"based on today's exchange rate: 0.789206, 100.000000 USD is equivalent to approximately 78.920600 GBP","tool_call_id":"call_mgGM9fqGHTtUueokUa7uwYHT","function_name":"fn-exchange-rates","arguments":"{\"amount\": 100, \"source\": \"USD\", \"target\": \"GBP\"}"}

event:result
data: {"req_id":"7YU0SY","result":"133139.226800","retrieval_result":"based on today's exchange rate: 1331.392268, 100.000000 USD is equivalent to approximately 133139.226800 KRW","tool_call_id":"call_1IFlbtKNC5CEN13tBSM0Nson","function_name":"fn-exchange-rates","arguments":"{\"amount\": 100, \"source\": \"USD\", \"target\": \"KRW\"}"}
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
