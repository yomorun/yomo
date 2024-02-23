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
Content-Type: text/event-stream
Date: Sat, 17 Feb 2024 09:25:36 GMT
Transfer-Encoding: chunked

data: {"result":"2024-02-15 23:00:00","arguments":"{\"sourceTimezone\": \"America/Los_Angeles\", \"targetTimezone\": \"Asia/Singapore\", \"timeString\": \"2024-02-15 07:00:00\"}"}

data: {"result":"2024-02-16 00:00:00","arguments":"{\"sourceTimezone\": \"America/Los_Angeles\", \"targetTimezone\": \"Asia/Singapore\", \"timeString\": \"2024-02-15 08:00:00\"}"}
```

```bash
curl -i -X POST -H "Content-Type: application/json" -d '{"prompt":"How much is 100 US dollar in Korea and UK currency"}' http://127.0.0.1:8000/invoke
HTTP/1.1 200 OK
Transfer-Encoding: chunked
Connection: keep-alive
Content-Type: text/event-stream
Date: Fri, 23 Feb 2024 14:35:41 GMT
Keep-Alive: timeout=4
Proxy-Connection: keep-alive

data: 133258.000000

retrievalData: based on today's exchange rate: 1332.580000, 100.000000 USD is equivalent to approximately 133258.000000 KRW

data: 79.352500

retrievalData: based on today's exchange rate: 0.793525, 100.000000 USD is equivalent to approximately 79.352500 GBP
```
