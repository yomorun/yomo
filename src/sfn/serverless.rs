use std::path::{Path, absolute};
use std::process::Stdio;
use std::sync::Arc;

use anyhow::{Ok, Result, anyhow, bail};
use colored::Colorize;
use log::{debug, info};
use tempfile::tempdir;
use tokio::fs;
use tokio::{
    io::{AsyncBufReadExt, BufReader},
    process::Command,
    sync::RwLock,
};

use crate::connector::TcpConnector;

static GO_MAIN: &str = include_str!(concat!(
    env!("CARGO_MANIFEST_DIR"),
    "/serverless/go/main.go"
));
static GO_MOD: &str = include_str!(concat!(env!("CARGO_MANIFEST_DIR"), "/serverless/go/go.mod"));

/// Serverless function handler (supports Go)
#[derive(Default, Clone)]
pub struct ServerlessHandler {
    socket_addr: Arc<RwLock<Option<String>>>,
}

impl ServerlessHandler {
    /// Run serverless function as subprocess
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

        info!("serverless function exited");

        Ok(())
    }

    /// Get TCP connector to serverless function
    pub(crate) async fn get_connector(&self) -> Result<Option<TcpConnector>> {
        let socket_addr = self.socket_addr.read().await.clone();
        if let Some(addr) = socket_addr {
            return Ok(Some(TcpConnector::new(&addr)));
        }
        Ok(None)
    }

    /// Compile and run Go serverless function
    async fn run_go(&self, serverless_dir: &Path) -> Result<()> {
        // create temp directory for serverless function
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

        info!("starting serverless function");

        // start a sub process to run serverless
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
        let mut buf = String::new();
        if reader.read_line(&mut buf).await? == 0 {
            bail!("failed to read socket address from serverless process");
        }

        let addr = buf.trim().to_string();
        if addr.is_empty() {
            bail!("received empty socket address");
        }
        info!("serverless listening: {}", addr);

        *self.socket_addr.write().await = Some(addr);

        drop(temp_dir);

        loop {
            let mut buf = String::new();
            if reader.read_line(&mut buf).await? == 0 {
                break;
            }
            print!("{} {}", "[Go Serverless]".cyan(), buf);
        }
        child.wait().await?;

        Ok(())
    }
}
