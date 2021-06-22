### Example: Transfer binary data over YoMo

#### 1. Start YoMo Server

```bash
yomo serve -c zipper.yaml
```

#### 2. Hook the streaming serverless function to YoMo

```bash
yomo run func.go -n cc
```

#### 3. Generating binary data

```bash
go run source.go
```