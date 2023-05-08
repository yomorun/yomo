#[yomo::init]
fn init() -> anyhow::Result<Vec<u32>> {
    // return observe datatags
    Ok(vec![0x33])
}

#[yomo::handler]
fn handler(ctx: yomo::Context) -> anyhow::Result<()> {
    // load input tag & data
    let tag = ctx.get_tag();
    let input = ctx.load_input();
    println!("wasm rust sfn received {} bytes[{:#x}]", input.len(), tag);

    // parse input from bytes
    let input = String::from_utf8(input.to_vec())?;

    // your app logic goes here
    let output = input.to_uppercase();

    // return the datatag and output bytes
    ctx.dump_output(0x34, output.into_bytes());

    Ok(())
}
