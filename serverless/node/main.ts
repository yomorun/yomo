import { createServer, Socket } from "net"
import { join } from "path"
import * as tsj from "ts-json-schema-generator"

type RequestHeaders = {
  name: string
  trace_id: string
  span_id: string
  body_format: string
  extension: string
}

type RequestBody = {
  args: string
  agent_context?: string
}

type ResponseHeaders = {
  status_code: number
  error_msg: string
  body_format: string
  extension: string
}

type ResponseBody = {
  result?: unknown
  error_msg?: string
}

type ToolModule = {
  description?: string
  handler?: (args: unknown, agentContext?: Record<string, string>) => unknown | Promise<unknown>
}

function writeFrame(socket: Socket, packet: unknown): void {
  const payload = Buffer.from(JSON.stringify(packet), "utf8")
  const header = Buffer.allocUnsafe(4)
  header.writeUInt32BE(payload.length, 0)
  socket.write(header)
  socket.write(payload)
}

async function waitReadable(socket: Socket): Promise<void> {
  await new Promise<void>((resolve, reject) => {
    const onReadable = () => {
      cleanup()
      resolve()
    }
    const onEnd = () => {
      cleanup()
      reject(new Error("socket ended"))
    }
    const onClose = () => {
      cleanup()
      reject(new Error("socket closed"))
    }
    const onError = (err: Error) => {
      cleanup()
      reject(err)
    }
    const cleanup = () => {
      socket.off("readable", onReadable)
      socket.off("end", onEnd)
      socket.off("close", onClose)
      socket.off("error", onError)
    }

    socket.on("readable", onReadable)
    socket.on("end", onEnd)
    socket.on("close", onClose)
    socket.on("error", onError)
  })
}

function generateParametersSchema(appTsPath: string): unknown {
  try {
    const generator = tsj.createGenerator({
      path: appTsPath,
      tsconfig: join(__dirname, "..", "tsconfig.json"),
      type: "Argument",
      topRef: false,
      expose: "none",
      additionalProperties: false,
      skipTypeCheck: true,
      jsDoc: "extended",
    })

    const schema = generator.createSchema("Argument") as {
      $ref?: string
      definitions?: Record<string, unknown>
      [k: string]: unknown
    }

    if (typeof schema.$ref === "string" && schema.$ref.startsWith("#/definitions/")) {
      const key = schema.$ref.slice("#/definitions/".length)
      const deref = schema.definitions?.[key]
      if (deref != null) {
        return deref
      }
    }

    return schema
  } catch {
    return {
      type: "object",
      properties: {},
      additionalProperties: false,
    }
  }
}

async function handleConnection(socket: Socket, toolModule: ToolModule): Promise<void> {
  socket.pause()

  let buffer = Buffer.alloc(0)
  const ensureBuffered = async (bytes: number): Promise<void> => {
    while (buffer.length < bytes) {
      const chunk = socket.read() as Buffer | null
      if (chunk != null) {
        buffer = Buffer.concat([buffer, chunk])
        continue
      }

      await waitReadable(socket)
    }
  }

  const readPacket = async <T>(): Promise<T> => {
    await ensureBuffered(4)
    const length = buffer.readUInt32BE(0)
    await ensureBuffered(4 + length)

    const packetBuf = buffer.subarray(4, 4 + length)
    buffer = buffer.subarray(4 + length)
    return JSON.parse(packetBuf.toString("utf8")) as T
  }

  try {
    const reqHeaders = await readPacket<RequestHeaders>()
    if (reqHeaders.body_format !== "bytes") {
      writeFrame(socket, {
        status_code: 400,
        error_msg: "unsupported body format",
        body_format: "null",
        extension: "",
      } satisfies ResponseHeaders)
      socket.end()
      return
    }

    const reqBody = await readPacket<RequestBody>()
    const args = reqBody.args ? JSON.parse(reqBody.args) : {}
    const agentContext = reqBody.agent_context
      ? (JSON.parse(reqBody.agent_context) as Record<string, string>)
      : undefined

    let result: unknown
    let error_msg = ""
    try {
      if (typeof toolModule.handler !== "function") {
        throw new Error("handler is not exported")
      }
      result = await toolModule.handler(args, agentContext)
    } catch (err) {
      error_msg = err instanceof Error ? err.message : String(err)
    }

    writeFrame(socket, {
      status_code: 200,
      error_msg: "",
      body_format: "bytes",
      extension: "",
    } satisfies ResponseHeaders)
    writeFrame(socket, {
      result,
      ...(error_msg ? { error_msg } : {}),
    } satisfies ResponseBody)
    socket.end()
  } catch (err) {
    writeFrame(socket, {
      status_code: 400,
      error_msg: err instanceof Error ? err.message : String(err),
      body_format: "null",
      extension: "",
    } satisfies ResponseHeaders)
    socket.end()
  }
}

async function main(): Promise<void> {
  const toolModule = (await import("./src/app.js")) as ToolModule
  const description = typeof toolModule.description === "string" ? toolModule.description : ""
  const appTsPath = join(__dirname, "..", "src", "app.ts")
  const parameters = generateParametersSchema(appTsPath)

  const schema = {
    description,
    parameters,
  }

  const server = createServer({ allowHalfOpen: true }, (socket) => {
    void handleConnection(socket, toolModule)
  })

  await new Promise<void>((resolve, reject) => {
    server.once("error", reject)
    server.listen(0, "127.0.0.1", () => {
      resolve()
    })
  })

  const addr = server.address()
  if (typeof addr !== "object" || addr == null) {
    throw new Error("invalid listen address")
  }

  process.stdout.write(`YOMO_TOOL_JSONSCHEMA: ${JSON.stringify(schema)}\n`)
  process.stdout.write(`YOMO_TOOL_ADDR: ${addr.address}:${addr.port}\n`)

  process.stdin.resume()
  process.stdin.on("end", () => {
    process.exit(0)
  })
}

void main().catch((err) => {
  process.stderr.write(`${err instanceof Error ? err.stack ?? err.message : String(err)}\n`)
  process.exit(1)
})
