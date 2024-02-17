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
cd sfn-currency-exchange && go run main.go
```

## Step 3: Invoke the LLM Function

```bash
curl -X POST -H "Content-Type: application/json" -d '{"prompt":"tell me the time in Singapore, based on the time provided: Thursday, February 15th, 2024 7:00am to 8:00am (UTC-08:00) Pacific Time - Los Angeles?"}' http://127.0.0.1:8000/invoke
```

```bash
curl -X POST -H "Content-Type: application/json" -d '{"prompt":"How much is 100 US dollar in China currency"}' http://127.0.0.1:8000/invoke
```
