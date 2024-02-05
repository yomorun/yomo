## Example 10: Target property and Cron feature

### Steps to run the example

1. Start Zipper Server

```bash
yomo serve -c ../config.yaml
```

2. Start `sfn-1-executor`

This stateful serverless function will emit data every 2 seconds.

```bash
go run sfn-1-executor/main.go
```

3. Start two instances of `sfn-2-sink` to consume the data

First one start with `USERID=alice`, this instance will consume the data with `target` property set to `alice`.

```bash
USERID=alice go run sfn-2-sink/main.go
```

Second one start with `USERID=bob`, this instance will consume the data with `target` property set to `bob`.

```bash
USERID=bob go run sfn-2-sink/main.go
```
