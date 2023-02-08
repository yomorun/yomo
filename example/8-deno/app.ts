import { Request, Response, run } from "https://deno.land/x/yomo/mod.ts";

const enc = new TextEncoder();
const dec = new TextDecoder();

async function handler(req: Request): Promise<Response> {
  console.log("deno sfn received %d bytes", req.data.length);
  return new Response(0x34, enc.encode(dec.decode(req.data).toUpperCase()));
}

await run([0x33], handler);
