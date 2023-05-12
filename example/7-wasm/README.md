# Implement YoMo Stream Function by WebAssembly

YoMo is capable of running compiled [WebAssembly](https://webassembly.org)
serverless functions, which means developers can use their familiar programming
languages other than Go to implement YoMo applications.

**Notice**: _The wasm serverless API is an experimental feature currently, so
feedback is highly welcomed and there may be changes in the future stable
releases._

## WASM/WASI Runtimes

Currently, YoMo support three popular wasm runtimes:

- [wazero](https://wazero.io)
- [Wasmtime](https://wasmtime.dev)
- [WasmEdge](https://wasmedge.org)

By default, wasm stream functions are served by [wazero](https://wazero.io),
which is a zero dependency WebAssembly runtime written in Go.

Also, YoMo integrated [Wasmtime](https://wasmtime.dev) and
[WasmEdge](https://wasmedge.org). Developers can choose your favorite one as the
runtime. But both Wasmtime and WasmEdge need to install their requirements:

- Wasmtime

  Install via homebrew:

  ```sh
  brew install wasmtime
  ```

  Or run the install script: https://docs.wasmtime.dev/cli-install.html

- WasmEdge

  Run the install script: https://wasmedge.org/book/en/#install

  Or download from [releases](https://github.com/wasmedge/wasmedge/releases),
  and extract to `/usr/local/{include,lib,bin}`.

## Run the demo example

1. Build a wasm stream function

   You can choose a specific language example from the following list, and then
   follow the instructions step by step to compile a wasm file (string
   upper-case):

   - [Rust](sfn/rust/README.md)
   - [Go](sfn/go/README.md)
   - [C](sfn/c/README.md)
   - [Zig](sfn/zig/README.md)

   We will continue adding more examples for other languages (Python, C#,
   Kotlin, etc.).

2. Run the complete application

   In this demo, the source will keep sending a string (`Hello, YoMo!`). The
   wasm serverless function will observe the source data, and send the processed
   data to the next streaming function. The whole data flow is:

   `Source (original string) --> Wasm-SFN (convert to upper-case) --> SFN-2 (print result)`

- Start YoMo zipper

  ```sh
  yomo serve -c ../uppercase/config.yaml
  ```

- Start wasm serverless function

  ```sh
  cd sfn
  yomo run sfn.wasm
  ```

  you can indicate the wasm runtime via the `--runtime` parameter (`wasmtime` |
  `wasmedge`), the default value is `wasmtime`.

- Start Source & Sink

  ```sh
  cd ../uppercase/source
  go run main.go
  ```

## Wasm serverless development

In the above examples, we have shown the basic usage of writing a wasm
serverless function. Now we will elaborate on our wasm development design.

1. Import functions

   These functions are injected into the wasm environment at the initialization
   period.

   - `yomo_observe_datatag: [tag I32] -> []`

     Declare observing a datatag by this serverless function.

   - `yomo_context_data_size: [] -> [tag I32]`

     This function will return the input data size.

   - `yomo_context_data: [pointer I32, length I32] -> []`

     This function aims to load the input data from source or upstream SFN node,
     hence it should be called at the very beginning in the `yomo_handler`.
     Developers should allocate a continuous memory buffer space, with passing
     the start buffer position as the `pointer` parameter and memory buffer size
     as `length` parameter.

   - `yomo_write: [tag I32, pointer I32, length I32] -> []`

     Similarly, this function is used for passing the output data to the host
     environment. Notice that it can be executed multiple times.

2. Export functions

   - `yomo_init: [] -> []`

     You can do the initialization tasks in this function, such as loading a
     config file.

   - `yomo_handler: [] -> []`

     This is the essential feature of the serverless functions: processing
     application data.

3. File system

   The WASI runtime is designed as a sandbox container. For this reason, the
   file system access inside the wasm environment is strictly restricted to the
   current working directory.

4. Environment variables

   Env-vars are inherited from the host environment. It will be convenient to
   load configs in a wasm application simply with your env-vars:
   `MYAPP_FOO=abc yomo run foo.wasm`.

5. Std IO

   Similar to the env-vars, standard input and output streams are also inherited
   from the host environment, so you can print logs to the console just like
   writing native programs.
