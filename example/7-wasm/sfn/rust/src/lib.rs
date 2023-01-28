#[yomo::init]
fn init() -> anyhow::Result<Vec<u32>> {
    // return observe datatags
    Ok(vec![0x33])
}

#[yomo::handler]
fn handler(input: &[u8]) -> anyhow::Result<(u32, Vec<u8>)> {
    println!("wasm rust sfn received {} bytes", input.len());

    // parse input from bytes
    let input = String::from_utf8(input.to_vec())?;

    // your app logic goes here
    let output = input.to_uppercase();

    // return the datatag and output bytes
    Ok((0x34, output.into_bytes()))
}
