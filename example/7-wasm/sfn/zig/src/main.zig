const std = @import("std");
const allocator = std.heap.page_allocator;

extern fn yomo_observe_datatag(tag: u32) void;
extern fn yomo_context_tag() u32;
extern fn yomo_context_data(pointer: *const u8, size: u32) u32;
extern fn yomo_context_data_size() u32;
extern fn yomo_write(tag: u32, pointer: *const u8, length: usize) u32;

pub fn main() !void {
    std.log.info("yomo wasm sfn on zig", .{});
}

export fn yomo_init() void {
    yomo_observe_datatag(0x33);
}

export fn yomo_init_fn() u32 {
    std.log.info("wasm zig sfn init", .{});
    return 0;
}

export fn yomo_handler() void {
    // load input data
    const tag = yomo_context_tag();
    const size: u32 = yomo_context_data_size();
    const input = allocator.alloc(u8, size) catch undefined;
    _ = yomo_context_data(&input[0], size);
    defer allocator.free(input);
    std.log.info("wasm zig sfn received {d} bytes with 0x{x}", .{ input.len, tag });

    // process app data
    var output = std.ascii.allocUpperString(allocator, input) catch undefined;
    defer allocator.free(output);

    // dump output data
    _ = yomo_write(0x34, &output[0], output.len);
}
