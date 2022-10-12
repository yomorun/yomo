import { Reader, Writer } from "https://deno.land/std@0.155.0/io/types.d.ts";
import {
  putVarnum,
  readVarnum,
} from "https://deno.land/std@0.155.0/encoding/binary.ts";

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

async function readData(reader: Reader): Promise<Uint8Array | null> {
  const length = await readVarnum(reader, {
    "dataType": "uint32",
    "endian": "little",
  });
  const buf = new Uint8Array(length);
  const n = await reader.read(buf);
  if (n == null || n !== length) {
    return null;
  }
  return buf;
}

async function writeData(writer: Writer, data: Uint8Array) {
  const length = new Uint8Array(4);
  putVarnum(length, data.length, {
    "dataType": "uint32",
    "endian": "little",
  });
  await writer.write(length);
  await writer.write(data);
}

export async function run(
  observed: [number],
  handler: (req: Request) => Response,
) {
  let sock = "./sfn.sock";
  if (Deno.args.length > 0) {
    sock = Deno.args[0];
  }

  const conn = await Deno.connect({
    path: sock,
    transport: "unix",
  });

  await writeData(conn, Uint8Array.from(observed));
  for (;;) {
    const buf = await readData(conn);
    if (buf == null) {
      break;
    }

    const req = new Request(buf);
    const res = handler(req);

    conn.write(Uint8Array.from([res.tag]));
    await writeData(conn, res.data);
  }

  conn.close();
}
