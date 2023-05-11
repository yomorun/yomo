import { Context, run } from "../../cli/serverless/deno/mod/mod.ts";
// In your own applications change the above line to:
// import { Context, run } from "https://deno.land/x/yomo/mod.ts";

const enc = new TextEncoder();
const dec = new TextDecoder();

async function handler(ctx: Context) {
  console.log(
    "deno sfn received %d bytes with tag[%d]",
    ctx.input.length,
    ctx.tag,
  );
  const output = dec.decode(ctx.input).toUpperCase();
  await ctx.write(0x34, enc.encode(output));
}

await run([0x33], handler);
