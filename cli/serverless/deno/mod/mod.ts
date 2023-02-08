import { Reader, Writer } from "https://deno.land/std/types.d.ts";
import {
  readVarnum,
  varnumBytes,
  VarnumOptions,
} from "https://deno.land/std/encoding/binary.ts";
import { loadSync } from "https://deno.land/std/dotenv/mod.ts";

export class Request {
  data: Uint8Array;

  constructor(data: Uint8Array) {
    this.data = data;
  }
}

export class Response {
  tag: number;
  data: Uint8Array;

  constructor(tag: number, data: Uint8Array) {
    this.tag = tag;
    this.data = data;
  }
}

const VARNUM_OPTIONS: VarnumOptions = {
  "dataType": "uint32",
  "endian": "little",
};

function numberToBytes(val: number): Uint8Array {
  return varnumBytes(val, VARNUM_OPTIONS);
}

async function readData(reader: Reader): Promise<Uint8Array | null> {
  let length = 0;
  try {
    length = await readVarnum(reader, VARNUM_OPTIONS);
  } catch (e) {
    if (e instanceof Deno.errors.UnexpectedEof) {
      return null;
    }
    throw e;
  }
  const buf = new Uint8Array(length);
  const n = await reader.read(buf);
  if (n == null || n !== length) {
    return null;
  }
  return buf;
}

async function writeData(writer: Writer, data: Uint8Array) {
  await writer.write(numberToBytes(data.length));
  await writer.write(data);
}

export async function run(
  observed: [number],
  handler: (req: Request) => Promise<Response>,
) {
  let sock = "./sfn.sock";
  let env = null;
  if (Deno.args.length > 0) {
    sock = Deno.args[0];
    if (Deno.args.length > 1) {
      env = Deno.args[1];
    }
  }

  if (env != null) {
    loadSync({
      envPath: env,
      defaultsPath: "",
      examplePath: "",
      export: true,
      allowEmptyValues: true,
    });
  }

  const conn: Deno.UnixConn = await Deno.connect({
    path: sock,
    transport: "unix",
  });

  await conn.write(numberToBytes(observed.length));
  for (const tag of observed) {
    await conn.write(numberToBytes(tag));
  }

  for (;;) {
    const buf = await readData(conn);
    if (buf == null) {
      break;
    }

    const req = new Request(buf);
    const res = await handler(req);

    await conn.write(numberToBytes(res.tag));
    await writeData(conn, res.data);
  }

  conn.close();
}
