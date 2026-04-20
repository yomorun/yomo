use std::{path::Path, process, sync::Arc};

use anyhow::{Result, anyhow, bail};
use clap::{Parser, builder::NonEmptyStringValueParser};
use config::{Config, File};
use env_logger::Env;
use log::{error, info};
use serde::Deserialize;
use tokio::{
    fs,
    net::TcpListener,
    select, spawn,
    sync::mpsc::unbounded_channel,
    time::{Duration, sleep},
};

use yomo::{
    auth::AuthImpl,
    bridge::Bridge,
    client::Client,
    connector::MemoryConnector,
    llm_api::build_llm_api,
    metadata_mgr::MetadataMgrImpl,
    router::RouterImpl,
    serverless::{ServerlessHandler, ServerlessMemoryBridge},
    tls::TlsConfig,
    tool_api::build_tool_api,
    tool_mgr::ToolMgrImpl,
    zipper::{MemorySource, Zipper, ZipperBridge},
};

const MAX_BUF_SIZE: usize = 64 * 1024;

/// Default host address
fn default_host() -> String {
    "127.0.0.1".to_string()
}

/// Default Zipper QUIC port
fn default_zipper_port() -> u16 {
    9000
}

/// Default Http API HTTP port
fn default_http_api_port() -> u16 {
    9001
}

/// Default LLM base URL
fn default_llm_base_url() -> String {
    "http://127.0.0.1:11434".to_string()
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
struct LlmConfig {
    #[serde(default = "default_llm_base_url")]
    base_url: String,

    #[serde(default)]
    api_key: String,
}

impl Default for LlmConfig {
    fn default() -> Self {
        Self {
            base_url: default_llm_base_url(),
            api_key: String::new(),
        }
    }
}

#[derive(Debug, Clone, Deserialize)]
struct HttpApiConfig {
    #[serde(default = "default_host")]
    host: String,

    #[serde(default = "default_http_api_port")]
    port: u16,

    #[serde(default)]
    llm: LlmConfig,

    #[serde(default)]
    enable_tool_api: bool,
}

impl Default for HttpApiConfig {
    fn default() -> Self {
        Self {
            host: default_host(),
            port: default_http_api_port(),
            llm: LlmConfig::default(),
            enable_tool_api: false,
        }
    }
}

/// Server configuration
#[derive(Debug, Clone, Deserialize, Default)]
#[serde(default, rename_all = "snake_case")]
struct ServeConfig {
    zipper: ZipperConfig,

    http_api: HttpApiConfig,
}

/// CLI commands
#[derive(Parser, Debug)]
#[command(author, version, about, long_about = None)]
enum Cli {
    /// Serve a YoMo Service (Zipper)
    Serve(ServeOptions),

    /// Initialize a YoMo Tool project
    Init(InitOptions),

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

    #[clap(
        short,
        long,
        value_parser = NonEmptyStringValueParser::new(),
        env="YOMO_TOOL_NAME",
        help = "the serverless tool name"
    )]
    name: String,

    #[clap(
        short,
        long,
        env = "YOMO_ZIPPER",
        default_value = "127.0.0.1:9000",
        help = "YoMo-Zipper endpoint address"
    )]
    zipper: String,

    #[clap(
        short,
        long,
        env = "YOMO_CREDENTIAL",
        default_value = "",
        help = "client credential payload"
    )]
    credential: String,

    #[clap(
        long,
        env = "YOMO_TLS_CA_CERT_FILE",
        help = "path to the tls CA certificate file"
    )]
    tls_ca_cert_file: Option<String>,

    #[clap(
        long,
        env = "YOMO_TLS_CERT_FILE",
        help = "path to the tls client certificate file (for mutual TLS mode)"
    )]
    tls_cert_file: Option<String>,

    #[clap(
        long,
        env = "YOMO_TLS_KEY_FILE",
        help = "path to the tls client key file (for mutual TLS mode)"
    )]
    tls_key_file: Option<String>,

    #[clap(
        long,
        env = "YOMO_TLS_MUTUAL",
        help = "option to enable mutual TLS mode"
    )]
    tls_mutual: bool,

    #[clap(
        short,
        long,
        env = "YOMO_TOOL_LANGUAGE",
        value_parser = ["node", "go"],
        help = "tool language: node/go (auto-detect when omitted)"
    )]
    language: Option<String>,
}

/// Init command options
#[derive(Parser, Debug)]
struct InitOptions {
    #[clap(
        short,
        long,
        default_value = "node",
        value_parser = ["node", "go"],
        help = "tool language template"
    )]
    language: String,

    #[clap(
        default_value = "./app",
        help = "directory to initialize the tool project"
    )]
    output_dir: String,
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

    let (sender, receiver) = unbounded_channel();
    let tool_mgr = Arc::new(ToolMgrImpl::new());
    let zipper = Zipper::builder()
        .auth(Arc::new(AuthImpl::new(config.zipper.auth_token)))
        .metadata_mgr(Arc::new(MetadataMgrImpl::new()))
        .router(Arc::new(RouterImpl::new()))
        .tool_mgr(tool_mgr.clone())
        .build();
    let zipper_memory_bridge = ZipperBridge::new(zipper.clone(), MemorySource::new(receiver), ());
    let connector = MemoryConnector::new(sender.clone(), MAX_BUF_SIZE);

    let mut app = axum::Router::new().nest(
        "/v1",
        build_llm_api(
            connector.to_owned(),
            tool_mgr,
            config.http_api.llm.base_url,
            config.http_api.llm.api_key,
        )
        .await?,
    );
    if config.http_api.enable_tool_api {
        app = app.nest("/tool", build_tool_api(connector).await?);
    }
    info!(
        "start HTTP API server on {}:{} (LLM API at /v1, Tool API {})",
        config.http_api.host,
        config.http_api.port,
        if config.http_api.enable_tool_api {
            "enabled at /tool"
        } else {
            "disabled"
        }
    );
    let listener = TcpListener::bind((config.http_api.host.as_ref(), config.http_api.port)).await?;

    select! {
        _ = zipper_memory_bridge.serve_bridge() => Ok(()),
        r = zipper.serve(&config.zipper.host, config.zipper.port, &config.zipper.tls) => r,
        r = axum::serve(listener, app) => r.map_err(|e| anyhow!(e)),
    }?;

    Ok(())
}

/// Initialize serverless tool project
async fn init(opt: InitOptions) -> Result<()> {
    let output_dir = Path::new(&opt.output_dir);
    if output_dir.exists() {
        let mut entries = fs::read_dir(output_dir).await?;
        if entries.next_entry().await?.is_some() {
            bail!("output directory is not empty: {:?}", output_dir);
        }
    } else {
        fs::create_dir_all(output_dir).await?;
    }

    match opt.language.as_str() {
        "node" => {
            ServerlessHandler::init_node(output_dir).await?;
            info!("initialized node tool project: {}", output_dir.display());
            info!("next step: edit {}/src/app.ts", output_dir.display());
        }
        "go" => {
            ServerlessHandler::init_go(output_dir).await?;
            info!("initialized go tool project: {}", output_dir.display());
            info!("next step: edit {}/app.go", output_dir.display());
        }
        _ => unreachable!(),
    }

    Ok(())
}

/// Run serverless tool
async fn run(opt: RunOptions) -> Result<()> {
    let language = match opt.language.as_deref() {
        Some("go") => yomo::serverless::ServerlessLanguage::Go,
        Some("node") => yomo::serverless::ServerlessLanguage::Node,
        None => yomo::serverless::ServerlessLanguage::Auto,
        Some(_) => unreachable!(),
    };

    let tls_config = TlsConfig::builder()
        .maybe_ca_cert(opt.tls_ca_cert_file)
        .maybe_cert(opt.tls_cert_file)
        .maybe_key(opt.tls_key_file)
        .mutual(opt.tls_mutual)
        .build();

    let (sender, receiver) = unbounded_channel();
    let serverless_handler = ServerlessHandler::default();
    let serverless_memory_bridge =
        ServerlessMemoryBridge::new(serverless_handler.clone(), receiver);

    let run_handler = serverless_handler.clone();
    let serverless_dir = opt.serverless_dir.clone();
    let run_task =
        spawn(async move { run_handler.run_subprocess(&serverless_dir, language).await });

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

    // Clean up subprocess if connection fails
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
    env_logger::Builder::from_env(Env::default().default_filter_or("info")).init();

    if let Err(e) = match Cli::parse() {
        Cli::Serve(opt) => serve(opt).await,
        Cli::Init(opt) => init(opt).await,
        Cli::Run(opt) => run(opt).await,
    } {
        error!("{}", e);
        process::exit(1);
    }
}
