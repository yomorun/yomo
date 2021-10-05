# YoMo Example 1: Linux Pipeline over cloud

## 1. Prepare

## 2. Run program

### Start the Zipper service:

`yomo serve -c workflow.yaml`

![yomo example 1: linux pipeline, zipper](https://docs.yomo.run/1.5/1-zipper.png)

### Start the Streaming Serverless to observe data:

`yomo run -n rand serverless/rand.go`

![yomo example 1: linux pipeline, build streaming function](https://docs.yomo.run/1.5/2-sfn1.png)

after few seconds, build is success, you should see the following:

![yomo example 1: linux pipeline, build streaming function](https://docs.yomo.run/1.5/2-sfn2.png)

### Start the Source to generate random data and send to Zipper:

`cat /dev/urandom | go run source/pipe.go`

![yomo example 1: linux pipeline, start source to emit data](https://docs.yomo.run/1.5/3-source.png)
