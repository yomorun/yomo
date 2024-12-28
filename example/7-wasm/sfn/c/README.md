# C wasm serverless function

## Install wasi-sdk

download from https://github.com/WebAssembly/wasi-sdk/releases

## Build

```sh
# specify the wasi-sdk directory path according to your system
export WASI_SDK_PATH=~/Downloads/wasi-sdk-25.0-arm64-macos

$WASI_SDK_PATH/bin/clang --target=wasm32-wasip1 \
    --sysroot=$WASI_SDK_PATH/share/wasi-sysroot \
    -nostartfiles -fvisibility=hidden -O3 \
    -Wl,--no-entry,--export=yomo_init,--export=yomo_handler,--export=yomo_observe_datatags \
    -o sfn.wasm sfn.c

cp sfn.wasm ..
```
