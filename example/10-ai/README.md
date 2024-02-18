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
Date: Sat, 17 Feb 2024 09:25:17 GMT
Transfer-Encoding: chunked

data: {"result":"79.591100","arguments":"{\"amount\": 100, \"target\": \"GBP\"}"}

data: {"result":"132857.602200","arguments":"{\"amount\": 100, \"target\": \"KRW\"}"}
```
