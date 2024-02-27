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
Date: Fri, 23 Feb 2024 15:52:39 GMT
Keep-Alive: timeout=4
Proxy-Connection: keep-alive

event:result
data: {"req_id":"vafZVB","arguments":"{\"sourceTimezone\": \"America/Los_Angeles\", \"targetTimezone\": \"Asia/Singapore\", \"timeString\": \"2024-02-15 07:00:00\"}","result":"2024-02-15 23:00:00","retrieval_result":"The time in timezone Asia/Singapore is 2024-02-15 23:00:00","tool_call_id":"call_MMxZsUduPvATWCoGQGb16O68","function_name":"fn-timezone-converter"}

event:retrieval_result
data: The time in timezone Asia/Singapore is 2024-02-15 23:00:00

event:result
data: {"req_id":"vafZVB","arguments":"{\"sourceTimezone\": \"America/Los_Angeles\", \"targetTimezone\": \"Asia/Singapore\", \"timeString\": \"2024-02-15 08:00:00\"}","result":"2024-02-16 00:00:00","retrieval_result":"The time in timezone Asia/Singapore is 2024-02-16 00:00:00","tool_call_id":"call_uH09734Ct2s19PAnAORT4VlK","function_name":"fn-timezone-converter"}

event:retrieval_result
data: The time in timezone Asia/Singapore is 2024-02-16 00:00:00
```

```bash
curl -i -X POST -H "Content-Type: application/json" -d '{"prompt":"How much is 100 US dollar in Korea and UK currency"}' http://127.0.0.1:8000/invoke
HTTP/1.1 200 OK
Transfer-Encoding: chunked
Connection: keep-alive
Content-Type: text/event-stream
Date: Fri, 23 Feb 2024 15:52:13 GMT
Keep-Alive: timeout=4
Proxy-Connection: keep-alive

event:result
data: {"req_id":"PpKvms","arguments":"{\"amount\": 100, \"target\": \"KRW\"}","result":"133258.000000","retrieval_result":"based on today's exchange rate: 1332.580000, 100.000000 USD is equivalent to approximately 133258.000000 KRW","tool_call_id":"call_fW5MX6GXnirT2kVhyskid7cL","function_name":"fn-exchange-rates"}

event:retrieval_result
data: based on today's exchange rate: 1332.580000, 100.000000 USD is equivalent to approximately 133258.000000 KRW

event:result
data: {"req_id":"PpKvms","arguments":"{\"amount\": 100, \"target\": \"GBP\"}","result":"79.352500","retrieval_result":"based on today's exchange rate: 0.793525, 100.000000 USD is equivalent to approximately 79.352500 GBP","tool_call_id":"call_hThpXYSXfi4ViMAzBJ6HLnsU","function_name":"fn-exchange-rates"}

event:retrieval_result
data: based on today's exchange rate: 0.793525, 100.000000 USD is equivalent to approximately 79.352500 GBP
```
