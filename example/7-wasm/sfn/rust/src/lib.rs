extern "C" {
    fn yomo_observe_datatag(tag: u8);
    fn yomo_load_input(pointer: *mut u8);
    fn yomo_dump_output(tag: u8, pointer: *const u8, length: usize);
}

#[no_mangle]
pub extern "C" fn yomo_init() {
    unsafe {
        yomo_observe_datatag(0x33);
    }
}

#[no_mangle]
pub extern "C" fn yomo_handler(input_length: usize) {
    println!("wasm rust sfn received {} bytes", input_length);

    // load input data
    let mut input = Vec::with_capacity(input_length);
    unsafe {
        yomo_load_input(input.as_mut_ptr());
        input.set_len(input_length);
    }

    // process app data
    let output = input.to_ascii_uppercase();

    // dump output data
    unsafe {
        yomo_dump_output(0x34, output.as_ptr(), output.len());
    }
}
