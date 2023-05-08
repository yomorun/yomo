//! # YoMo Rust development sdk
//!
//! This crate is designed for developers to implementing their own YoMo applications with Rust language.

/// Serverless handler function context
pub struct Context {}

extern "C" {
    fn yomo_context_tag() -> u32;
    fn yomo_context_data_size() -> usize;
    fn yomo_context_data(pointer: *mut u8, length: usize) -> usize;
    fn yomo_write(tag: u32, pointer: *const u8, length: usize) -> i32;
}

impl Context {
    /// Returns the data tag
    pub fn get_tag(&self) -> u32 {
        unsafe { yomo_context_tag() }
    }

    /// Loads the input data from the wasm host
    pub fn load_input(&self) -> Vec<u8> {
        let length = unsafe { yomo_context_data_size() };
        let mut input = Vec::with_capacity(length);
        unsafe {
            yomo_context_data(input.as_mut_ptr(), length);
            input.set_len(length);
        }
        input
    }

    /// Dumps output data (this function can be executed multiple times)
    pub fn dump_output(&self, tag: u32, output: Vec<u8>) {
        unsafe {
            yomo_write(tag, output.as_ptr(), output.len());
        }
    }
}

pub use yomo_derive::{handler, init};
