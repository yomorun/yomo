import * as log from "https://deno.land/std@0.161.0/log/mod.ts";
import {
  Request,
  Response,
  run,
} from "https://deno.land/x/yomo@v1.10.0/mod.ts";

const enc = new TextEncoder();
const dec = new TextDecoder();

function handler(req: Request): Response {
  log.info({ runtime: "sfn-deno", size: req.data.length });

  return new Response(0x34, enc.encode(dec.decode(req.data).toUpperCase()));
}

await run([0x33], handler);
