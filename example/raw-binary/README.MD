### Example: Transfer binary data over YoMo

#### 1. Start YoMo Server

```bash
yomo serve -c yomo.yaml
```

#### 2. Hook the streaming serverless function to YoMo

```bash
yomo run -n cc func.go
```

#### 3. Generating binary data

```bash
go run source.go
```