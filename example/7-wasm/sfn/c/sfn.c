#include <ctype.h>
#include <stdio.h>
#include <stdlib.h>

__attribute__((import_module("env"),
               import_name("yomo_observe_datatag"))) extern void
observe(uint32_t tag);

__attribute__((import_module("env"),
               import_name("yomo_context_tag"))) extern uint32_t
get_tag();

__attribute__((import_module("env"),
               import_name("yomo_context_data_size"))) extern size_t
get_input_size();

__attribute__((import_module("env"),
               import_name("yomo_context_data"))) extern size_t
load_input(char *pointer, size_t length);

__attribute__((import_module("env"), import_name("yomo_write"))) extern int32_t
dump_output(uint32_t tag, const char *pointer, size_t length);

void yomo_init() { observe(0x33); }

uint32_t yomo_init_fn() {
  printf("wasm c sfn init\n");
  return 0;
}

void yomo_handler() {
  // load input tag & data
  uint32_t tag = get_tag();
  size_t length = get_input_size();
  char *input = malloc(length);
  load_input(input, length);
  printf("wasm c sfn received %zu bytes with tag[%#x]\n", length, tag);

  // process app data
  char *output = malloc(length);
  for (size_t i = 0; i < length; i++) {
    output[i] = toupper(input[i]);
  }

  // dump output data
  dump_output(0x34, output, length);

  free(input);
  free(output);
}
