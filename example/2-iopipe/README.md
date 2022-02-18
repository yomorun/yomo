# YoMo Example 2: Linux Pipeline over cloud

In Unix-like computer operating systems, a [pipeline](<https://en.wikipedia.org/wiki/Pipeline_(Unix)>) is a mechanism for inter-process communication using message passing. A pipeline is a set of processes chained together by their standard streams, so that the output text of each process (stdout) is passed directly as input (stdin) to the next one. The second process is started as the first process is still executing, and they are executed concurrently. The concept of pipelines was championed by Douglas McIlroy at Unix's ancestral home of Bell Labs, during the development of Unix, shaping its [toolbox philosophy](https://en.wikipedia.org/wiki/Unix_philosophy)

![yomo example 1: unix pipeline](https://docs.yomo.run/1.5/the-linux-programming-interface.png)

Dennis Ritchie, the creator of the Unix operating system, introduced the concept of a pipeline to process data.

> In a new version of the Unix operating system, a flexible coroutine-based design replaces the traditional rigid connection between processes and terminals or networks. Processing modules may be inserted dynamically into the stream that connects a user's program to a device. Programs may also connect directly to programs, providing inter-process communication.

[AT&T Bell Laboratories Technical Journal 63, No. 8 Part 2 (October, 1984)](https://www.bell-labs.com/usr/dmr/www/st.html)

Nowadays, our software deployed on the cloud and serve people from all over the world. Building a complex geo-distributed system to provide secure and reliable services with low-latency is a challenge.

By introducting [YoMo](https://yomo.run), we can build it just like `unix pipeline over cloud`.

## 1. Prepare

Install YoMo CLI

### Binary (Recommended)

```bash
$ curl -fsSL "https://bina.egoist.sh/yomorun/cli?name=yomo" | sh

  ==> Resolved version latest to v0.1.7
  ==> Downloading asset for darwin amd64
  ==> Installing yomo to /usr/local/bin
  ==> Installation complete
```

### Or build from source

```bash
$ go install github.com/yomorun/cli/yomo@latest
$ yomo version
YoMo CLI Version: v0.1.7
```

## 2. Run program

### Start the Zipper service:

`yomo serve -c workflow.yaml`

### Start the Streaming Function to observe data:

`yomo run serverless/counter.go -n counter`

![yomo example 1: unix pipeline, build streaming function](https://docs.yomo.run/1.5/2-sfn1.png)

after few seconds, build is success, you should see the following:

![yomo example 1: unix pipeline, build streaming function](https://docs.yomo.run/1.5/2-sfn2.png)

### Start the Source to generate random data and send to Zipper:

`cat /dev/urandom | go run source/pipe.go`

![yomo example 1: unix pipeline, start source to emit data](https://docs.yomo.run/1.5/3-source.png)
