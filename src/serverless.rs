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
static NODE_MAIN: &str = include_str!(concat!(
    env!("CARGO_MANIFEST_DIR"),
    "/serverless/node/main.ts"
));
static NODE_PACKAGE_JSON: &str = include_str!(concat!(
    env!("CARGO_MANIFEST_DIR"),
    "/serverless/node/package.json"
));
static NODE_TSCONFIG: &str = include_str!(concat!(
    env!("CARGO_MANIFEST_DIR"),
    "/serverless/node/tsconfig.json"
));

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum ServerlessLanguage {
    Auto,
    Go,
    Node,
}

/// Tool subprocess handler (supports Go and Node TypeScript).
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

    fn detect_language(&self, serverless_dir: &Path) -> Option<ServerlessLanguage> {
        if serverless_dir.join("app.go").exists() {
            Some(ServerlessLanguage::Go)
        } else if serverless_dir.join("src/app.ts").exists()
            || serverless_dir.join("app.ts").exists()
        {
            Some(ServerlessLanguage::Node)
        } else {
            None
        }
    }

    /// Run tool as subprocess.
    pub async fn run_subprocess(
        &self,
        serverless_dir: &str,
        language: ServerlessLanguage,
    ) -> Result<()> {
        // get the absolute path of serverless directory
        let serverless_dir = Path::new(serverless_dir);
        if !serverless_dir.is_dir() {
            bail!("{} is not a directory", serverless_dir.display());
        }
        let serverless_dir = absolute(serverless_dir)?;

        info!("start to run serverless tool: {}", serverless_dir.display());

        let selected_language = match language {
            ServerlessLanguage::Auto => self.detect_language(&serverless_dir).ok_or(anyhow!(
                "unsupported serverless source in {}: expected app.go or src/app.ts",
                serverless_dir.display()
            ))?,
            forced => forced,
        };

        match selected_language {
            ServerlessLanguage::Go => {
                if !serverless_dir.join("app.go").exists() {
                    bail!("app.go not found in {}", serverless_dir.display());
                }
                self.run_go(&serverless_dir).await?;
            }
            ServerlessLanguage::Node => {
                if !serverless_dir.join("src/app.ts").exists()
                    && !serverless_dir.join("app.ts").exists()
                {
                    bail!("app.ts not found in {}", serverless_dir.display());
                }
                self.run_node_typescript(&serverless_dir).await?;
            }
            ServerlessLanguage::Auto => unreachable!(),
        }

        info!("tool exited");

        Ok(())
    }

    async fn copy_dir_recursive(&self, src: &Path, dst: &Path) -> Result<()> {
        fs::create_dir_all(dst).await?;

        let mut stack = vec![(src.to_path_buf(), dst.to_path_buf())];
        while let Some((current_src, current_dst)) = stack.pop() {
            fs::create_dir_all(&current_dst).await?;
            let mut entries = fs::read_dir(&current_src).await?;
            while let Some(entry) = entries.next_entry().await? {
                let entry_type = entry.file_type().await?;
                let src_path = entry.path();
                let dst_path = current_dst.join(entry.file_name());
                if entry_type.is_dir() {
                    stack.push((src_path, dst_path));
                } else if entry_type.is_file() {
                    fs::copy(src_path, dst_path).await?;
                }
            }
        }

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
            print!("{} {}", "[Go Serverless]".cyan(), buf);
        }
        child.wait().await?;

        Ok(())
    }

    /// Compile and run Node TypeScript tool
    async fn run_node_typescript(&self, serverless_dir: &Path) -> Result<()> {
        let temp_dir = tempdir()?;
        let cwd = temp_dir.path();
        debug!("cwd: {}", cwd.display());

        fs::write(cwd.join("main.ts"), NODE_MAIN).await?;

        if serverless_dir.join("package.json").exists() {
            fs::copy(
                serverless_dir.join("package.json"),
                cwd.join("package.json"),
            )
            .await?;
            if serverless_dir.join("package-lock.json").exists() {
                fs::copy(
                    serverless_dir.join("package-lock.json"),
                    cwd.join("package-lock.json"),
                )
                .await?;
            }
        } else {
            fs::write(cwd.join("package.json"), NODE_PACKAGE_JSON).await?;
        }

        fs::write(cwd.join("tsconfig.json"), NODE_TSCONFIG).await?;

        if serverless_dir.join("src").exists() {
            self.copy_dir_recursive(&serverless_dir.join("src"), &cwd.join("src"))
                .await?;
        } else {
            fs::create_dir_all(cwd.join("src")).await?;
        }

        if !cwd.join("src/app.ts").exists() {
            if serverless_dir.join("app.ts").exists() {
                fs::copy(serverless_dir.join("app.ts"), cwd.join("src/app.ts")).await?;
            } else {
                bail!("app.ts not found in {}", serverless_dir.display());
            }
        }

        if serverless_dir.join(".env").exists() {
            fs::copy(serverless_dir.join(".env"), cwd.join(".env")).await?;
        }

        let install_cmd = if cwd.join("package-lock.json").exists() {
            vec!["ci"]
        } else {
            vec!["install"]
        };
        let install_output = Command::new("npm")
            .args(install_cmd)
            .current_dir(cwd)
            .spawn()?
            .wait_with_output()
            .await?;
        if !install_output.status.success() {
            bail!(
                "npm dependency install failed: {}",
                String::from_utf8_lossy(&install_output.stderr)
            );
        }

        let runtime_deps_output = Command::new("npm")
            .args([
                "install",
                "--no-save",
                "typescript",
                "typescript-json-schema",
                "@types/node",
            ])
            .current_dir(cwd)
            .spawn()?
            .wait_with_output()
            .await?;
        if !runtime_deps_output.status.success() {
            bail!(
                "npm runtime dependency install failed: {}",
                String::from_utf8_lossy(&runtime_deps_output.stderr)
            );
        }

        let build_output = Command::new("npx")
            .args([
                "tsc",
                "main.ts",
                "src/app.ts",
                "--outDir",
                "dist",
                "--module",
                "commonjs",
                "--target",
                "es2020",
                "--moduleResolution",
                "node",
                "--esModuleInterop",
                "--strict",
                "--skipLibCheck",
            ])
            .current_dir(cwd)
            .spawn()?
            .wait_with_output()
            .await?;
        if !build_output.status.success() {
            bail!(
                "typescript build failed: {}",
                String::from_utf8_lossy(&build_output.stderr)
            );
        }

        info!("starting tool");
        let mut child = Command::new("node")
            .args(["dist/main.js"])
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
                print!("{} {}", "[Node Serverless]".cyan(), buf);
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
            print!("{} {}", "[Node Serverless]".cyan(), buf);
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
