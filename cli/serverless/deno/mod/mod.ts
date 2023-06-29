import { Reader, Writer } from "https://deno.land/std/types.d.ts";
import {
  readVarnum,
  varnumBytes,
  VarnumOptions,
} from "https://deno.land/std/encoding/binary.ts";
import { loadSync } from "https://deno.land/std/dotenv/mod.ts";

export class Context {
  tag: number;
  input: Uint8Array;
  private writer: Writer;

  constructor(tag: number, input: Uint8Array, writer: Writer) {
    this.tag = tag;
    this.input = input;
    this.writer = writer;
  }

  async write(tag: number, data: Uint8Array) {
    await this.writer.write(numberToBytes(tag));
    await writeData(this.writer, data);
  }
}

const VARNUM_OPTIONS: VarnumOptions = {
  "dataType": "uint32",
  "endian": "little",
};

function numberToBytes(val: number): Uint8Array {
  return varnumBytes(val, VARNUM_OPTIONS);
}

async function readNumber(reader: Reader): Promise<number | null> {
  try {
    return await readVarnum(reader, VARNUM_OPTIONS);
  } catch (e) {
    if (e instanceof Deno.errors.UnexpectedEof) {
      return null;
    }
    throw e;
  }
}

async function readData(reader: Reader): Promise<Uint8Array | null> {
  const length = await readNumber(reader);
  if (length == null) {
    return null;
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
  handler: (ctx: Context) => Promise<void>,
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
    const tag = await readNumber(conn);
    if (tag == null) {
      break;
    }

    const data = await readData(conn);
    if (data == null) {
      break;
    }

    const ctx = new Context(tag, data, conn);
    await handler(ctx);

    await conn.write(numberToBytes(0)); // tag
    await conn.write(numberToBytes(0)); // length
  }

  conn.close();
}
