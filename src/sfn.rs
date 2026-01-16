use anyhow::{Result, anyhow, bail};
use bon::Builder;
use log::error;
use s2n_quic::{Client, Connection, client::Connect, stream::BidirectionalStream};
use std::{net::ToSocketAddrs, path::Path};
use tokio::io::{AsyncReadExt, AsyncWriteExt};

use crate::{
    frame::{Frame, HandshakePayload, read_frame, write_frame},
    types::SfnRequest,
};

#[derive(Clone, Builder)]
pub struct Sfn {
    #[builder(default = String::from("localhost:9000"))]
    zipper: String,

    #[builder(default)]
    credential: String,

    sfn_name: String,
}

impl Sfn {
    pub async fn serve(self) -> Result<()> {
        // Create QUIC client with TLS certificate and listening address
        let tls = s2n_quic::provider::tls::default::Client::builder()
            .with_certificate(Path::new("cert.pem"))?
            .with_application_protocols(&["yomo-v2"])?
            .build()?;

        let client = Client::builder()
            .with_tls(tls)?
            .with_io("0.0.0.0:0")?
            .start()?;

        // Connect to zipper service
        let (server_name, server_port) = self
            .zipper
            .split_once(':')
            .ok_or_else(|| anyhow!("invalid zipper addr format"))?;
        let server_port = server_port.parse::<u16>()?;
        let addr = (server_name, server_port)
            .to_socket_addrs()?
            .next()
            .ok_or_else(|| anyhow!("no zipper ip found"))?;
        let mut conn = client
            .connect(Connect::new(addr).with_server_name(server_name))
            .await?;
        conn.keep_alive(true)?;
        println!("connected to zipper");

        self.handle_connection(conn).await
    }

    // Handle QUIC connection
    async fn handle_connection(&self, mut conn: Connection) -> Result<()> {
        // Open control stream and register function name
        let mut ctrl_stream = conn.open_bidirectional_stream().await?;

        // Send handshake request
        self.handshake(&mut ctrl_stream).await?;

        // Accept and process data streams (zipper creates new streams for each request)
        while let Some(stream) = conn.accept_bidirectional_stream().await? {
            let sfn = self.clone();
            tokio::spawn(async move {
                if let Err(e) = sfn.handle_data_stream(stream).await {
                    eprintln!("handle_data_stream error: {}", e);
                }
            });
        }

        Ok(())
    }

    // Send handshake request
    async fn handshake(&self, ctrl_stream: &mut BidirectionalStream) -> Result<()> {
        let h = Frame::Handshake {
            payload: HandshakePayload {
                sfn_name: self.sfn_name.to_owned(),
                credential: self.credential.to_owned(),
                ..Default::default()
            },
        };

        write_frame(ctrl_stream, &h).await?;

        match read_frame(ctrl_stream).await? {
            Frame::HandshakeAck { payload } => {
                if !payload.ok {
                    error!("handshake failed: {}", payload.reason.unwrap_or_default());
                    bail!("handshake failed");
                }
            }
            _ => bail!("unexpected frame"),
        };

        Ok(())
    }

    // Handle data stream: receive request, execute processing, return response
    async fn handle_data_stream(&self, stream: BidirectionalStream) -> Result<()> {
        let stream_id = stream.id();
        println!("new data stream: {}", stream_id);

        let (mut receive_stream, mut send_stream) = stream.split();

        // Read request parameters
        let mut buf = Vec::new();
        receive_stream.read_to_end(&mut buf).await?;
        let req: SfnRequest = serde_json::from_slice(&buf)?;

        // Call function handler
        let resp = self.call_handler(&req.args, &req.context).await?;

        // Send response and close stream
        send_stream.write_all(resp.as_bytes()).await?;
        send_stream.close().await?;
        println!("stream closed: {}", stream_id);

        Ok(())
    }

    // Function handler: actually execute function logic
    async fn call_handler(&self, args: &str, context: &str) -> Result<String> {
        println!("args: {}, {}", args, context);

        let resp = args.to_ascii_uppercase();

        Ok(resp)
    }
}
