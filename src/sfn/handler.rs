use std::path::{Path, absolute};
use std::process::Stdio;
use std::time::Duration;

use anyhow::{Result, bail};
use log::{debug, info};
use tempfile::tempdir;
use tokio::fs;
use tokio::{
    io::{AsyncBufReadExt, BufReader, ReadHalf, SimplexStream, WriteHalf},
    net::TcpStream,
    process::Command,
    sync::Mutex,
    time::timeout,
};

use crate::io::pipe_stream;

static GO_MAIN: &str = include_str!(concat!(
    env!("CARGO_MANIFEST_DIR"),
    "/serverless/go/main.go"
));
static GO_MOD: &str = include_str!(concat!(env!("CARGO_MANIFEST_DIR"), "/serverless/go/go.mod"));

#[async_trait::async_trait]
pub trait Handler: Send + Sync {
    async fn forward(
        &self,
        reader: ReadHalf<SimplexStream>,
        writer: WriteHalf<SimplexStream>,
    ) -> Result<()>;
}

#[derive(Default)]
pub struct ServerlessHandler {
    socket_addr: Mutex<String>,
}

impl ServerlessHandler {
    pub async fn run_subprocess(&self, serverless_dir: &str) -> Result<()> {
        // get the absolute path of serverless directory
        let serverless_dir = Path::new(serverless_dir);
        if !serverless_dir.is_dir() {
            bail!("{} is not a directory", serverless_dir.display());
        }
        let serverless_dir = absolute(serverless_dir)?;

        info!(
            "start to run serverless: {}",
            serverless_dir.display().to_string()
        );

        // find app.go in serverless directory
        if !serverless_dir.join("app.go").exists() {
            bail!("app.go not found in {}", serverless_dir.display());
        }

        self.run_go(&serverless_dir).await?;

        Ok(())
    }

    async fn run_go(&self, serverless_dir: &Path) -> Result<()> {
        // create temp directory for serverless function
        let temp_dir = tempdir()?;
        debug!("temp_dir: {}", temp_dir.path().display());

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

        info!("starting serverless function");

        // start a sub process to run serverless
        let mut child = Command::new("go")
            .args(["run", "."])
            .current_dir(cwd)
            .stdin(Stdio::piped())
            .stdout(Stdio::piped())
            .stderr(Stdio::piped())
            .spawn()?;

        let mut buf = String::with_capacity(256);
        let n = timeout(
            Duration::from_secs(10),
            BufReader::new(child.stdout.as_mut().expect("Failed to open stdout"))
                .read_line(&mut buf),
        )
        .await??;

        if n == 0 {
            bail!("failed to read socket address from serverless process");
        }

        let addr = buf.trim().to_string();
        if addr.is_empty() {
            bail!("received empty socket address");
        }
        println!("serverless listening: {}", addr);

        *self.socket_addr.lock().await = addr;

        drop(temp_dir);

        loop {
            let mut buf = String::with_capacity(256);
            let n: usize = BufReader::new(child.stderr.as_mut().expect("Failed to open stderr"))
                .read_line(&mut buf)
                .await?;
            println!("{}", buf.trim());

            if n == 0 {
                break;
            }
        }

        child.wait().await?;

        Ok(())
    }
}

#[async_trait::async_trait]
impl Handler for ServerlessHandler {
    async fn forward(
        &self,
        reader: ReadHalf<SimplexStream>,
        writer: WriteHalf<SimplexStream>,
    ) -> Result<()> {
        let socket_addr = self.socket_addr.lock().await.clone();
        if socket_addr.is_empty() {
            bail!("serverless process is not started");
        }

        let mut stream = TcpStream::connect(&socket_addr).await?;
        let (to_reader, to_writer) = stream.split();

        // Pipe data between streams
        pipe_stream(reader, writer, to_reader, to_writer).await;

        Ok(())
    }
}
