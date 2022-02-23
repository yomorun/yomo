# YoMo Example 2: Linux Pipeline over cloud

In Unix-like computer operating systems, a [pipeline](<https://en.wikipedia.org/wiki/Pipeline_(Unix)>) is a mechanism for inter-process communication using message passing. A pipeline is a set of processes chained together by their standard streams, so that the output text of each process (stdout) is passed directly as input (stdin) to the next one. The second process is started as the first process is still executing, and they are executed concurrently. The concept of pipelines was championed by Douglas McIlroy at Unix's ancestral home of Bell Labs, during the development of Unix, shaping its [toolbox philosophy](https://en.wikipedia.org/wiki/Unix_philosophy)

![yomo example 1: unix pipeline](https://docs.yomo.run/1.5/the-linux-programming-interface.png)

Dennis Ritchie, the creator of the Unix operating system, introduced the concept of a pipeline to process data.

> In a new version of the Unix operating system, a flexible coroutine-based design replaces the traditional rigid connection between processes and terminals or networks. Processing modules may be inserted dynamically into the stream that connects a user's program to a device. Programs may also connect directly to programs, providing inter-process communication.

[AT&T Bell Laboratories Technical Journal 63, No. 8 Part 2 (October, 1984)](https://www.bell-labs.com/usr/dmr/www/st.html)

Nowadays, our software deployed on the cloud and serve people from all over the world. Building a complex geo-distributed system to provide secure and reliable services with low-latency is a challenge.

By introducting [YoMo](https://yomo.run), we can build it just like `unix pipeline over cloud`.

## Prepare

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

## Option 1: Auto Run

```bash
$ task run
task: [sfn] yomo run -n counter serverless/counter.go
task: [source] cat /dev/urandom | go run source/pipe.go
task: [zipper] yomo serve -c workflow.yaml
[sfn] Using config file: workflow.yaml
[sfn] ℹ️   YoMo Stream Function file: serverless/counter.go
[sfn] ⌛  Create YoMo Stream Function instance...
[zipper] Using config file: workflow.yaml
[zipper] ℹ️   Running YoMo-Zipper...
[zipper] 2022-02-20 16:35:14.140	[yomo:zipper] Listening SIGUSR1, SIGUSR2, SIGTERM/SIGINT...
[zipper] 2022-02-20 16:35:14.148	[core:server] ✅ [example-pipeline] Listening on: [::]:9000, QUIC: [v1 draft-29], AUTH: [None]
[sfn] ℹ️   Starting YoMo Stream Function instance with Name: counter. Host: localhost. Port: 9000.
[sfn] ⌛  YoMo Stream Function building...
[source] 2022-02-20 16:35:14.552	[core:client] use credential: [None]
[source] 2022-02-20 16:35:14.558	[core:client] ❤️  [source-pipe]([::]:51817) is connected to YoMo-Zipper localhost:9000
[zipper] 2022-02-20 16:35:14.558	[core:server] ❤️  <Source> [::source-pipe](127.0.0.1:51817) is connected!

[iopipe:sfn] 2022-02-20 16:34:34.690	Got: 32768
[iopipe:sfn] 2022-02-20 16:34:34.690	Got: 32768
[iopipe:sfn] 2022-02-20 16:34:34.691	Got: 32768
[iopipe:sfn] 2022-02-20 16:34:34.691	Got: 32768
[iopipe:sfn] 2022-02-20 16:34:34.691	Got: 32768
[iopipe:sfn] 2022-02-20 16:34:34.692	Got: 32768
[iopipe:sfn] 2022-02-20 16:34:34.692	Got: 32768
[iopipe:sfn] 2022-02-20 16:34:34.693	Got: 32768
[iopipe:sfn] 2022-02-20 16:34:34.693	Got: 32768
```

## Option 2: Manual

First, start `Zipper` process:

`yomo serve -c workflow.yaml`

Then, start the Streaming Function to observe data:

`yomo run -n counter serverless/counter.go`

after few seconds, build is success, then, start the Source to generate random data and send to Zipper:

`cat /dev/urandom | go run source/pipe.go`
