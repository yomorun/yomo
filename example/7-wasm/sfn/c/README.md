# C wasm serverless function

## Install wasi-sdk

https://github.com/WebAssembly/wasi-sdk

## Build

```sh
# specify the wasi-sdk version and the directory path according to your system
export WASI_VERSION_FULL=16.0
export WASI_SDK_PATH=~/Downloads/wasi-sdk-$WASI_VERSION_FULL

$WASI_SDK_PATH/bin/clang --target=wasm32-unknown-wasi \
    --sysroot=$WASI_SDK_PATH/share/wasi-sysroot \
    -nostartfiles -fvisibility=hidden -O3 \
    -Wl,--no-entry,--export=yomo_init,--export=yomo_handler \
    -o sfn.wasm sfn.c

cp sfn.wasm ..
```
