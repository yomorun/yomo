//! # YoMo Rust development sdk (macros)
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
/// fn handler(ctx: yomo::Context) -> anyhow::Result<()> {
///     let input = ctx.load_input();
///     let output = String::from_utf8(input)?.to_uppercase();
///     ctx.dump_output(0x34, output.into_bytes());
///     Ok(())
/// }
/// ```
#[proc_macro_attribute]
pub fn handler(_args: TokenStream, input: TokenStream) -> TokenStream {
    let derive_input = &parse_macro_input!(input as ItemFn);
    let fn_name = &derive_input.sig.ident;

    let func: ItemFn = parse_quote! {
        #[no_mangle]
        pub extern "C" fn yomo_handler() {
            let ctx = yomo::Context{};

            #derive_input

            if let Err(e) = #fn_name(ctx) {
                eprintln!("sfn handler error: {}", e); // todo: export error
            }
        }
    };

    func.to_token_stream().into()
}
