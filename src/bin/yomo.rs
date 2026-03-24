use std::process;

use anyhow::{Result, anyhow};
use clap::{Parser, builder::NonEmptyStringValueParser};
use config::{Config, File};
use log::{error, info};
use serde::Deserialize;
use tokio::{
    select,
    sync::mpsc,
    time::{Duration, sleep},
};

use yomo::{
    bridge::Bridge,
    client::Client,
    connector::MemoryConnector,
    router::RouterImpl,
    serverless::{ServerlessHandler, ServerlessMemoryBridge},
    tls::TlsConfig,
    tool_api::serve_tool_api,
    zipper::{Zipper, ZipperMemoryBridge},
};

const MAX_BUF_SIZE: usize = 4 * 1024 * 1024;

/// Default host address
fn default_host() -> String {
    "127.0.0.1".to_string()
}

/// Default Zipper QUIC port
fn default_zipper_port() -> u16 {
    9000
}

/// Default tool API HTTP port
fn default_tool_api_port() -> u16 {
    9001
}

#[derive(Debug, Clone, Deserialize)]
struct ZipperConfig {
    #[serde(default = "default_host")]
    host: String,

    #[serde(default = "default_zipper_port")]
    port: u16,

    #[serde(default)]
    tls: TlsConfig,

    #[serde(default)]
    auth_token: Option<String>,
}

impl Default for ZipperConfig {
    fn default() -> Self {
        Self {
            host: default_host(),
            port: default_zipper_port(),
            tls: TlsConfig::default(),
            auth_token: None,
        }
    }
}

#[derive(Debug, Clone, Deserialize)]
struct ToolApiConfig {
    #[serde(default)]
    enabled: bool,

    #[serde(default = "default_host")]
    host: String,

    #[serde(default = "default_tool_api_port")]
    port: u16,
}

impl Default for ToolApiConfig {
    fn default() -> Self {
        Self {
            enabled: false,
            host: default_host(),
            port: default_tool_api_port(),
        }
    }
}

/// CLI commands
#[derive(Parser, Debug)]
#[command(author, version, about, long_about = None)]
enum Cli {
    /// Serve a YoMo Service (Zipper)
    Serve(ServeOptions),

    /// Run a YoMo Tool
    Run(RunOptions),
}

/// Serve command options
#[derive(Parser, Debug)]
struct ServeOptions {
    #[clap(short, long, help = "path to the YoMo server configuration file")]
    config: Option<String>,
}

/// Run command options
#[derive(Parser, Debug)]
struct RunOptions {
    #[clap(default_value = ".", help = "directory to the tool source file")]
    serverless_dir: String,

    #[clap(short, long, value_parser = NonEmptyStringValueParser::new(), help = "yomo tool name")]
    name: String,

    #[clap(
        short,
        long,
        default_value = "127.0.0.1:9000",
        help = "YoMo-Zipper endpoint address"
    )]
    zipper: String,

    #[clap(long, default_value_t = String::default(), help = "client credential payload")]
    credential: String,

    #[clap(long, help = "path to the tls CA certificate file")]
    tls_ca_cert_file: Option<String>,

    #[clap(
        long,
        help = "path to the tls client certificate file (for mutual TLS mode)"
    )]
    tls_cert_file: Option<String>,

    #[clap(long, help = "path to the tls client key file (for mutual TLS mode)")]
    tls_key_file: Option<String>,

    #[clap(long, default_value_t = false, help = "enable mutual TLS mode")]
    tls_mutual: bool,
}

/// Server configuration
#[derive(Debug, Clone, Deserialize)]
struct ServeConfig {
    #[serde(default)]
    zipper: ZipperConfig,

    #[serde(default, rename = "tool_api")]
    tool_api: ToolApiConfig,
}

impl Default for ServeConfig {
    fn default() -> Self {
        Self {
            zipper: ZipperConfig::default(),
            tool_api: ToolApiConfig::default(),
        }
    }
}

/// Start Zipper service
async fn serve(opt: ServeOptions) -> Result<()> {
    let config = match opt.config {
        Some(file) => {
            info!("load config file: {}", file);

            Config::builder()
                .add_source(File::with_name(&file))
                .build()?
                .try_deserialize::<ServeConfig>()?
        }
        None => {
            info!("use default config");

            ServeConfig::default()
        }
    };

    info!("config: {:?}", config);

    let (sender, receiver) = mpsc::unbounded_channel();
    let zipper = Zipper::new(RouterImpl::new(config.zipper.auth_token.clone()));
    let zipper_memory_bridge = ZipperMemoryBridge::new(zipper.clone(), receiver);

    select! {
        r = async {
            if config.tool_api.enabled {
                serve_tool_api(&config.tool_api.host, config.tool_api.port, MemoryConnector::new(sender, MAX_BUF_SIZE)).await
            } else {
                core::future::pending().await
            }
        } => r,
        _ = zipper_memory_bridge.serve_bridge() => Ok(()),
        r = zipper.serve(&config.zipper.host, config.zipper.port, &config.zipper.tls) => r,
    }?;

    Ok(())
}

/// Run serverless tool
async fn run(opt: RunOptions) -> Result<()> {
    let tls_config = TlsConfig::builder()
        .maybe_ca_cert(opt.tls_ca_cert_file)
        .maybe_cert(opt.tls_cert_file)
        .maybe_key(opt.tls_key_file)
        .mutual(opt.tls_mutual)
        .build();

    let (sender, receiver) = mpsc::unbounded_channel();
    let serverless_handler = ServerlessHandler::default();
    let serverless_memory_bridge =
        ServerlessMemoryBridge::new(serverless_handler.clone(), receiver);

    let run_handler = serverless_handler.clone();
    let serverless_dir = opt.serverless_dir.clone();
    let run_task = tokio::spawn(async move { run_handler.run_subprocess(&serverless_dir).await });

    let json_schema = loop {
        if let Some(schema) = serverless_handler.json_schema().await {
            break schema;
        }

        if run_task.is_finished() {
            let res = run_task
                .await
                .map_err(|e| anyhow!("tool subprocess task failed: {}", e))?;
            return match res {
                Ok(()) => Err(anyhow!(
                    "tool subprocess exited before startup metadata was ready"
                )),
                Err(e) => Err(e),
            };
        }

        sleep(Duration::from_millis(20)).await;
    };

    let mut client = Client::new(opt.name, Some(MemoryConnector::new(sender, MAX_BUF_SIZE)));
    if let Err(e) = client
        .connect_zipper(&opt.zipper, &opt.credential, &tls_config, Some(json_schema))
        .await
    {
        run_task.abort();
        return Err(e);
    }

    select! {
        r = async {
            run_task
                .await
                .map_err(|e| anyhow!("tool subprocess task failed: {}", e))?
        } => r,
        _ = serverless_memory_bridge.serve_bridge() => Ok(()),
        _ = client.serve_bridge() => Ok(()),
    }?;

    Ok(())
}

#[tokio::main]
async fn main() {
    env_logger::init();

    if let Err(e) = match Cli::parse() {
        Cli::Serve(opt) => serve(opt).await,
        Cli::Run(opt) => run(opt).await,
    } {
        error!("{}", e);
        process::exit(1);
    }
}
