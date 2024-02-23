# LLM Function Calling

## Step 1: Start the LLM Server

```bash
./../bin/yomo serve -c zipper.yaml
```

## Step 2: Start sfn

```bash
cd sfn-timezone-calculator && go run main.go
```

```bash
cd sfn-currency-converter && go run main.go
```

## Step 3: Invoke the LLM Function

```bash
curl -i -X POST -H "Content-Type: application/json" -d '{"prompt":"tell me the time in Singapore, based on the time provided: Thursday, February 15th, 2024 7:00am and 8:00am (UTC-08:00) Pacific Time"}' http://127.0.0.1:8000/invoke
HTTP/1.1 200 OK
Transfer-Encoding: chunked
Connection: keep-alive
Content-Type: text/event-stream
Date: Fri, 23 Feb 2024 15:14:17 GMT
Keep-Alive: timeout=4
Proxy-Connection: keep-alive

{"reqId":"7xtdhx","arguments":"{\"sourceTimezone\":\"America/Los_Angeles\",\"targetTimezone\":\"Asia/Singapore\",\"timeString\":\"2024-02-15 07:00:00\"}","result":"2024-02-15 23:00:00","retrievalResult":"The time in timezone Asia/Singapore is 2024-02-15 23:00:00","toolCallID":"call_r0xJ7mPT1Mra7AwQShHcbBjo","functionName":"fn-timezone-converter"}
{"retrievalData": "The time in timezone Asia/Singapore is 2024-02-15 23:00:00"}
```

```bash
curl -i -X POST -H "Content-Type: application/json" -d '{"prompt":"How much is 100 US dollar in Korea and UK currency"}' http://127.0.0.1:8000/invoke
HTTP/1.1 200 OK
Transfer-Encoding: chunked
Connection: keep-alive
Content-Type: text/event-stream
Date: Fri, 23 Feb 2024 15:15:16 GMT
Keep-Alive: timeout=4
Proxy-Connection: keep-alive

{"reqId":"jej-B4","arguments":"{\"amount\": 100, \"target\": \"GBP\"}","result":"79.352500","retrievalResult":"based on today's exchange rate: 0.793525, 100.000000 USD is equivalent to approximately 79.352500 GBP","toolCallID":"call_gkNzxJAfuYMVPaa5mRxeafNq","functionName":"fn-exchange-rates"}
{"retrievalData": "based on today's exchange rate: 0.793525, 100.000000 USD is equivalent to approximately 79.352500 GBP"}
{"reqId":"jej-B4","arguments":"{\"amount\": 100, \"target\": \"KRW\"}","result":"133258.000000","retrievalResult":"based on today's exchange rate: 1332.580000, 100.000000 USD is equivalent to approximately 133258.000000 KRW","toolCallID":"call_li6LAfkC2LYTF6OOMvaatOF5","functionName":"fn-exchange-rates"}
{"retrievalData": "based on today's exchange rate: 1332.580000, 100.000000 USD is equivalent to approximately 133258.000000 KRW"}
```
