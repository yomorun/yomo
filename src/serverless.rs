use std::path::{Path, absolute};
use std::process::Stdio;
use std::sync::Arc;

use anyhow::{Ok, Result, anyhow, bail};
use colored::Colorize;
use log::{debug, info};
use tempfile::tempdir;
use tokio::fs;
use tokio::io::{ReadHalf, SimplexStream, WriteHalf};
use tokio::net::tcp::{OwnedReadHalf, OwnedWriteHalf};
use tokio::sync::Mutex;
use tokio::sync::mpsc::UnboundedReceiver;
use tokio::{
    io::{AsyncBufReadExt, BufReader},
    process::Command,
    sync::RwLock,
};

use crate::bridge::Bridge;
use crate::connector::TcpConnector;
use crate::types::RequestHeaders;

static GO_MAIN: &str = include_str!(concat!(
    env!("CARGO_MANIFEST_DIR"),
    "/serverless/go/main.go"
));
static GO_MOD: &str = include_str!(concat!(env!("CARGO_MANIFEST_DIR"), "/serverless/go/go.mod"));

/// Tool subprocess handler (supports Go)
#[derive(Default, Clone)]
pub struct ServerlessHandler {
    socket_addr: Arc<RwLock<Option<String>>>,
    json_schema: Arc<RwLock<Option<String>>>,
}

impl ServerlessHandler {
    pub async fn socket_addr(&self) -> Option<String> {
        self.socket_addr.read().await.clone()
    }

    pub async fn json_schema(&self) -> Option<String> {
        self.json_schema.read().await.clone()
    }

    /// Run tool as subprocess
    pub async fn run_subprocess(&self, serverless_dir: &str) -> Result<()> {
        // get the absolute path of serverless directory
        let serverless_dir = Path::new(serverless_dir);
        if !serverless_dir.is_dir() {
            bail!("{} is not a directory", serverless_dir.display());
        }
        let serverless_dir = absolute(serverless_dir)?;

        info!("start to run serverless tool: {}", serverless_dir.display());

        // find app.go in serverless directory
        if !serverless_dir.join("app.go").exists() {
            bail!("app.go not found in {}", serverless_dir.display());
        }

        self.run_go(&serverless_dir).await?;

        info!("tool exited");

        Ok(())
    }

    /// Compile and run Go tool
    async fn run_go(&self, serverless_dir: &Path) -> Result<()> {
        // create temp directory for tool
        let temp_dir = tempdir()?;
        let cwd = temp_dir.path();
        debug!("cwd: {}", cwd.display());

        // write files to work directory
        fs::write(cwd.join("main.go"), GO_MAIN).await?;
        if serverless_dir.join("go.mod").exists() {
            fs::copy(serverless_dir.join("go.mod"), cwd.join("go.mod")).await?;
        } else {
            fs::write(cwd.join("go.mod"), GO_MOD).await?;
        }

        // copy app.go to work directory
        fs::copy(serverless_dir.join("app.go"), cwd.join("app.go")).await?;

        Command::new("go")
            .args(&["mod", "tidy"])
            .current_dir(cwd)
            .spawn()?
            .wait_with_output()
            .await?;

        info!("starting tool");

        // start a sub process to run tool
        let mut child = Command::new("go")
            .args(["run", "."])
            .current_dir(cwd)
            .stdin(Stdio::piped())
            .stdout(Stdio::piped())
            .spawn()?;

        let mut reader = BufReader::new(
            child
                .stdout
                .as_mut()
                .ok_or(anyhow!("Failed to open stdout"))?,
        );

        let mut got_addr = false;
        let mut got_schema = false;

        loop {
            let mut buf = String::new();
            if reader.read_line(&mut buf).await? == 0 {
                bail!("failed to read socket address from tool process: process exited");
            }

            let line = buf.trim();
            if let Some(stripped) = line.strip_prefix("YOMO_TOOL_JSONSCHEMA: ") {
                let json_schema = stripped.to_string();
                info!("tool json schema generated");
                *self.json_schema.write().await = Some(json_schema);
                got_schema = true;
            } else if let Some(stripped) = line.strip_prefix("YOMO_TOOL_ADDR: ") {
                let addr = stripped.to_string();
                info!("tool listening: {}", addr);
                *self.socket_addr.write().await = Some(addr);
                got_addr = true;
            } else if !line.is_empty() {
                print!("{} {}", "[Go Serverless]".cyan(), buf);
            }

            if got_addr && got_schema {
                break;
            }
        }

        loop {
            let mut buf = String::new();
            if reader.read_line(&mut buf).await? == 0 {
                break;
            }
            print!("{} {}", "[Go Tool]".cyan(), buf);
        }
        child.wait().await?;

        Ok(())
    }
}

#[derive(Clone)]
pub struct ServerlessMemoryBridge {
    handler: ServerlessHandler,
    receiver: Arc<Mutex<UnboundedReceiver<(ReadHalf<SimplexStream>, WriteHalf<SimplexStream>)>>>,
}

impl ServerlessMemoryBridge {
    pub fn new(
        handler: ServerlessHandler,
        receiver: UnboundedReceiver<(ReadHalf<SimplexStream>, WriteHalf<SimplexStream>)>,
    ) -> Self {
        Self {
            handler,
            receiver: Arc::new(Mutex::new(receiver)),
        }
    }
}

#[async_trait::async_trait]
impl
    Bridge<
        TcpConnector,
        ReadHalf<SimplexStream>,
        WriteHalf<SimplexStream>,
        OwnedReadHalf,
        OwnedWriteHalf,
    > for ServerlessMemoryBridge
{
    async fn accept(
        &mut self,
    ) -> Result<Option<(ReadHalf<SimplexStream>, WriteHalf<SimplexStream>)>> {
        Ok(self.receiver.lock().await.recv().await)
    }

    async fn find_downstream(
        &self,
        _req_headers: &Option<RequestHeaders>,
    ) -> Result<Option<TcpConnector>> {
        Ok(self
            .handler
            .socket_addr
            .read()
            .await
            .clone()
            .map(|addr| TcpConnector::new(&addr)))
    }
}
