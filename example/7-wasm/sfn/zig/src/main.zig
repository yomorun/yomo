const std = @import("std");
const allocator = std.heap.page_allocator;

const read_buf_size: u32 = 2048;
var read_buf: []u8 = undefined;
var read_buf_ptr: *const u8 = undefined;

extern fn yomo_observe_datatag(tag: u32) void;
extern fn yomo_context_tag() u32;
extern fn yomo_context_data(pointer: *const u8, size: u32) u32;
extern fn yomo_write(tag: u32, pointer: *const u8, length: usize) u32;

pub fn main() !void {
    std.log.info("yomo wasm sfn on zig", .{});
}

export fn yomo_init() void {
    read_buf = allocator.alloc(u8, read_buf_size) catch undefined;
    read_buf_ptr = &read_buf[0];
    yomo_observe_datatag(0x33);
}

export fn yomo_handler() void {
    // load input data
    const tag = yomo_context_tag();
    const input = getBytes(allocator, yomo_context_data);
    defer allocator.free(input);
    std.log.info("wasm zig sfn received {d} bytes with 0x{x}", .{ input.len, tag });

    // process app data
    var output = std.ascii.allocUpperString(allocator, input) catch undefined;
    // BUG: memory leaked
    // When we free the memory, we cannot write the content correctly.
    // defer allocator.free(output);

    // dump output data
    _ = yomo_write(0x34, &output[0], output.len);
    // std.debug.print("output[{*}] = {s}\n", .{ output, output });
}

// getBytes returns a byte slice of the given size
fn getBytes(a: std.mem.Allocator, comptime dataFn: fn (prt: *const u8, size: u32) callconv(.C) u32) []u8 {
    const size = dataFn(read_buf_ptr, read_buf_size);
    if (size == 0) {
        return undefined;
    }
    if (size > 0 and size <= read_buf_size) {
        // std.debug.print("read_buf = {s}\n", .{read_buf[0..size]});
        const result = a.alloc(u8, size) catch undefined;
        std.mem.copy(u8, result, read_buf[0..size]);
        return result;
    }
    // Otherwise, allocate a new buffer
    const buf2 = a.alloc(u8, size) catch undefined;
    _ = dataFn(&buf2[0], size);
    // std.debug.print("buf2 = {s}\n", .{buf2});
    return buf2;
}
