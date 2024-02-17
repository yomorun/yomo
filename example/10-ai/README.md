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
curl -i -X POST -H "Content-Type: application/json" -d '{"prompt":"tell me the time in Singapore, based on the time provided: Thursday, February 15th, 2024 7:00am to 8:00am (UTC-08:00) Pacific Time"}' http://127.0.0.1:8000/invoke
HTTP/1.1 200 OK
Content-Type: text/event-stream
Date: Sat, 17 Feb 2024 08:58:22 GMT
Content-Length: 27

data: 2024-02-15 23:00:00
```

```bash
curl -i -X POST -H "Content-Type: application/json" -d '{"prompt":"How much is 100 US dollar in Korea and UK currency"}' http://127.0.0.1:8000/invoke
HTTP/1.1 200 OK
Content-Type: text/event-stream
Date: Sat, 17 Feb 2024 08:48:47 GMT
Content-Length: 167

data: The exchange rate of KRW to USD is 1328.576022, compute result is 132857.602200

data: The exchange rate of GBP to USD is 0.795911, compute result is 79.591100
```
