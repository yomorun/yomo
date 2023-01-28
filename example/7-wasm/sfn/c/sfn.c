#include <ctype.h>
#include <stdio.h>
#include <stdlib.h>

__attribute__((import_module("env"), import_name("yomo_observe_datatag")))
extern void observe_datatag(uint32_t tag);

__attribute__((import_module("env"), import_name("yomo_load_input")))
extern void load_input(char *pointer);

__attribute__((import_module("env"), import_name("yomo_dump_output")))
extern void dump_output(uint32_t tag, const char *pointer, size_t length);

void yomo_init() {
    observe_datatag(0x33);
}

void yomo_handler(size_t input_length) {
    printf("wasm c sfn received %zu bytes\n", input_length);

    // load input data
    char *input = malloc(input_length);
    load_input(input);

    // process app data
    size_t output_length = input_length;
    char *output = malloc(output_length);
    for (size_t i = 0; i < input_length; i++) {
        output[i] = toupper(input[i]);
    }

    // dump output data
    dump_output(0x34, output, output_length);

    free(input);
    free(output);
}
