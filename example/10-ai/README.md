# LLM Function Calling

## Step 1: Start the LLM Server

```bash
./../bin/yomo serve -c zipper.yaml
```

## Step 2: Start sfn

```bash
cd llm-sfn-timezone-calculator && yomo run -m go.mod app.go
```

```bash
cd llm-sfn-currency-converter && yomo run -m go.mod app.go
```

```bash
cd llm-sfn-get-ip-and-latency && yomo run -m go.mod app.go
```


## Step 3: Invoke the LLM Function

```bash
$ curl -i -X POST -H "Content-Type: application/json" -d '{"prompt":"compare nike and puma website speed"}' http://127.0.0.1:8000/invoke
HTTP/1.1 200 OK
Content-Length: 944
Connection: keep-alive
Content-Type: application/json
Date: Tue, 19 Mar 2024 13:30:14 GMT
Keep-Alive: timeout=4
Proxy-Connection: keep-alive

{
  "Functions": null,
  "Content": "Based on the data provided for the domains nike.com and puma.com which include IP addresses and average latencies, we can infer the following about their website speeds:\n\n- Nike.com has an IP address of 13.225.183.84 with an average latency of 65.568333 milliseconds.\n- Puma.com has an IP address of 151.101.194.132 with an average latency of 54.563666 milliseconds.\n\nComparing these latencies, Puma.com is faster than Nike.com as it has a lower average latency. Please be aware, however, that website speed can be influenced by many factors beyond latency, such as server processing time, content size, and delivery networks among others. To get a more comprehensive understanding of website speed, you would need to consider additional metrics and possibly conductreal-time speed tests.",
  "ToolCalls": null,
  "FinishReason": "stop",
  "TokenUsage": {
    "prompt_tokens": 0,
    "completion_tokens": 0
  },
  "AssistantMessage": null
}
```

```bash
$ curl -i -X POST -H "Content-Type: application/json" -d '{"prompt":"what is the time in Singapore for Thursday, February 15th, 2024 7:00am and 8:00am (UTC-08:00) Pacific Time"}' http://127.0.0.1:8000/invoke
HTTP/1.1 200 OK
Content-Length: 618
Connection: keep-alive
Content-Type: application/json
Date: Tue, 19 Mar 2024 13:48:31 GMT
Keep-Alive: timeout=4
Proxy-Connection: keep-alive

{
  "Functions": null,
  "Content": "The converted times from UTC-08:00 (Pacific Time) to Singapore time for Thursday, February 15th, 2024, are as follows:\n\n- 7:00 am (Pacific Time) converts to 11:00 pm the same day in Singapore.\n- 8:00 am (Pacific Time) converts to 12:00 am (midnight) the following day, February 16th, 2024, in Singapore.\n\nPlease note that these conversions assume that there are no changes to the time zones or daylight saving schedules between now and the given date in 2024.",
  "ToolCalls": null,
  "FinishReason": "stop",
  "TokenUsage": {
    "prompt_tokens": 268,
    "completion_tokens": 122
  },
  "AssistantMessage": null
}
```

```bash
$ curl -i -X POST -H "Content-Type: application/json" -d '{"prompt":"How much is 100 usd in Korea and UK currency"}' http://127.0.0.1:8000/invoke
HTTP/1.1 200 OK
Content-Length: 333
Connection: keep-alive
Content-Type: application/json
Date: Tue, 19 Mar 2024 14:01:55 GMT
Keep-Alive: timeout=4
Proxy-Connection: keep-alive

{
  "Functions": null,
  "Content": "Based on today's exchange rate:\n\n- 100 USD is equivalent to approximately 133,909.72 South Korean Won (KRW).\n- 100 USD is equivalent to approximately 78.75 British Pounds (GBP).",
  "ToolCalls": null,
  "FinishReason": "stop",
  "TokenUsage": {
    "prompt_tokens": 213,
    "completion_tokens": 46
  },
  "AssistantMessage": null
}
```
