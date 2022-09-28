# YoMo CLI

YoMo 命令行工具

## 直接二进制文件

`curl -sL https://github.com/yomorun/yomo/releases/download/v1.1.1/yomo-v1.1.1-`uname -m`-`uname -s`.tar.gz | tar xvfz -`

或者

`curl -fsSL "https://bina.egoist.sh/yomorun/yomo?name=yomo" | sh`

## 基于源代码编译安装

❗️确保已安装 Go 编译运行环境，参考 [Installing Go](https://golang.org/doc/install)

## 安装
```sh
go install github.com/yomorun/yomo/cmd/yomo@latest
```

## 快速指南

### 1. Source 应用程序(数据来源)
#### 编写数据生产应用程序
参见 [example/source/main.go](example/source/main.go)

#### 运行 Source 应用

```
go run main.go
```

### 2. Stream Function 流处理函数
#### 初始化一个流处理函数 

```sh
yomo init [Name]
```

#### 运行流处理函数

```shell
cd [Name] && yomo run
```
生产环境
```sh
cd [Name] && yomo build && ./sl.yomo
```

### 3. Stream Function (数据输出)
#### 编写数据消费应用程序
参见 [example/stream-fn-db/app.go](example/stream-fn-db/app.go)

#### 运行 Output Connector 应用

```shell
cd example/stream-fn-db && yomo run
```
生产环境
```sh
cd example/stream-fn-db && yomo build && ./sl.yomo
```

### 4. YoMo-Zipper 应用编排
#### 编写工作流配置文件 `workflow.yaml`

```yaml
name: Service
host: localhost
port: 9000
functions:
  - name: Noise
```

#### 运行 YoMo-Zipper 应用程序

```shell
yomo serve --config workflow.yaml
```

## 示例

### 前置条件
- 安装 [task](https://taskfile.dev/#/installation)

### 运行示例

```shell
task example
```

