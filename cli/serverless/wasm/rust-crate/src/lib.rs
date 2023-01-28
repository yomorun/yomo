//! # YoMo Rust development sdk
//!
//! This crate is designed for developers to implementing their own YoMo applications with Rust language.

use proc_macro::TokenStream;
use quote::{quote, ToTokens};
use syn::{parse_macro_input, parse_quote, ItemFn};

/// You can do the initialization tasks in this function. The observed datatags should be returned.
///
/// # Examples
///
/// ```
/// #[yomo::init]
/// fn init() -> anyhow::Result<Vec<u32>> {
///     Ok(vec![0x33])
/// }
/// ```
#[proc_macro_attribute]
pub fn init(_args: TokenStream, input: TokenStream) -> TokenStream {
    let derive_input = &parse_macro_input!(input as ItemFn);
    let fn_name = &derive_input.sig.ident;

    let func: ItemFn = parse_quote! {
        #[no_mangle]
        pub extern "C" fn yomo_init() {
            #derive_input

            match #fn_name() {
                Ok(tags) => {
                    for tag in tags {
                        unsafe {
                            yomo_observe_datatag(tag);
                        }
                    }
                }
                Err(e) => eprintln!("sfn init error: {}", e), // todo: export error
            }
        }
    };

    let mut ts = quote! {
        extern "C" {
            fn yomo_observe_datatag(tag: u32);
            fn yomo_load_input(pointer: *mut u8);
            fn yomo_dump_output(tag: u32, pointer: *const u8, length: usize);
        }
    };

    ts.extend(func.to_token_stream().into_iter());

    ts.into()
}

/// This is the streaming data process handler for your app. Therefore it will be executed once a data packet is incoming.
///
/// # Examples
///
/// ```
/// #[yomo::handler]
/// fn handler(input: &[u8]) -> anyhow::Result<(u32, Vec<u8>)> {
///     let input = String::from_utf8(input.to_vec())?;
///     let output = input.to_uppercase();
///     Ok((0x34, output.into_bytes()));
/// }
/// ```
#[proc_macro_attribute]
pub fn handler(_args: TokenStream, input: TokenStream) -> TokenStream {
    let derive_input = &parse_macro_input!(input as ItemFn);
    let fn_name = &derive_input.sig.ident;

    let func: ItemFn = parse_quote! {
        #[no_mangle]
        pub extern "C" fn yomo_handler(input_length: usize) {
            let mut input = Vec::with_capacity(input_length);
            unsafe {
                yomo_load_input(input.as_mut_ptr());
                input.set_len(input_length);
            }

            #derive_input

            match #fn_name(&input) {
                Ok((tag, output)) => {
                    unsafe {
                        yomo_dump_output(tag, output.as_ptr(), output.len());
                    }
                }
                Err(e) => eprintln!("sfn handler error: {}", e), // todo: export error
            }
        }
    };

    func.to_token_stream().into()
}
