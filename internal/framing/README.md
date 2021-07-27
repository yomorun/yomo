# Framing

## Framing Format

```
    +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
    |                  Frame Length                 |
    +-----------------------------------------------+
    |                     Header                    |
    +-----------------------------------------------+
    |                      Data                     |
    +-----------------------------------------------+
```

* __Frame Length__: Unsigned 24-bit (24 bits = max value 2^24 = 16,777,215) integer represents the length of Frame in bytes. Excluding the Frame Length field.
* __Header__: The header of frame, it contains the `Frame Type` (required) and `Metadata` (optional).
* __Data__: The data of frame (optional).

## Frame Header Format

```
    +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
    |   Frame Type  |        Metadata Length        |
    +-----------------------------------------------+
    |                   Metadata                    |
    +-----------------------------------------------+
```

* __Frame Type__: Unsigned 8-bit integer represents the Frame Type. It's required for all frames.
* __Metadata Length__: Unsigned 16-bit (16 bits = max value 2^16 = 65,535) integer represents the length of Metadata in bytes. Excluding the Metadata Length field.
* __Metadata__: The metadata of frame (optional).

### Frame Types

|  Type                          | Value  | Description |
|:-------------------------------|:-------|:------------|
| __HANDSHAKE__                  | 0x00 | __Handshake__: Sent by client to initiate the connection with `YoMo-Zipper`.  |
| __HEARTBEAT__                  | 0x01 | __Heartbeat__: Connection heartbeat to check the health of peer. |
| __ACK__                        | 0x02 | __ACK__: Sent by `Stream Function` to acknowledge the data was received. |
| __ACCEPTED__                   | 0x03 | __Accepted__: Sent by `YoMo-Zipper` after handshake to inform the client that the connection setup was __accepted__. |
| __REJECTED__                   | 0x04 | __Rejected__: Sent by `YoMo-Zipper` after handshake to inform the client that the connection setup was __rejected__. |
| __CREATE_STREAM__              | 0x05 | __Create Stream__: Sent by `YoMo-Zipper` to inform the client to setup a QUIC stream. |
| __PAYLOAD__                    | 0x06 | __Payload__: Payload on a stream. |
| __INIT__                       | 0x07 | __Init__: Sent by client to initiate a stream after received the `CREATE_STREAM` frame from `YoMo-Zipper`. |
