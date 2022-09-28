const std = @import("std");

extern fn yomo_observe_datatag(tag: u8) void;
extern fn yomo_load_input(pointer: *const u8) void;
extern fn yomo_dump_output(tag: u8, pointer: *const u8, length: usize) void;

pub fn main() anyerror!void {
    std.log.info("yomo wasm sfn on zig", .{});
}

export fn yomo_init() void {
    yomo_observe_datatag(0x33);
}

export fn yomo_handler(input_length: usize) void {
    std.log.info("wasm zig sfn received {d} bytes", .{input_length});

    // load input data
    const allocator = std.heap.page_allocator;
    const input = allocator.alloc(u8, input_length) catch undefined;
    defer allocator.free(input);
    yomo_load_input(&input[0]);

    // process app data
    const output = std.ascii.allocUpperString(allocator, input) catch undefined;
    defer allocator.free(output);

    // dump output data
    yomo_dump_output(0x34, &output[0], output.len);
}
