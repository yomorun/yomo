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

    #[clap(
        short,
        long,
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

const NODE_TEMPLATE_APP: &str = r#"export const description = "Get weather for a city"

export type Argument = {
  /**
   * The city name to get the weather for.
   */
  city: string
}

export async function handler(args: Argument): Promise<string> {
  const city = (args.city || "").trim()
  if (!city) {
    throw new Error("city is required")
  }

  console.log(`query weather for city: ${city}`)

  const url = `https://wttr.in/${encodeURIComponent(city)}?format=3`
  const resp = await fetch(url)
  if (!resp.ok) {
    throw new Error(`failed to query weather, status code: ${resp.status}`)
  }

  const result = await resp.text()
  console.log(result)

  return result
}
"#;

const NODE_TEMPLATE_PACKAGE_JSON: &str = r#"{
  "name": "yomo-app",
  "private": true,
  "version": "0.0.1",
  "devDependencies": {
    "@types/node": "^22.10.1",
    "typescript": "^5.7.2"
  }
}
"#;

const NODE_TEMPLATE_TSCONFIG: &str = r#"{
  "compilerOptions": {
    "target": "es2020",
    "module": "commonjs",
    "moduleResolution": "node",
    "esModuleInterop": true,
    "strict": true,
    "skipLibCheck": true
  },
  "include": ["src/**/*.ts"]
}
"#;

const GO_TEMPLATE_APP: &str = r#"package main

import (
	"fmt"
	"log/slog"
	"io"
	"net/http"
)

const Description = "Get weather for a city"

type Arguments struct {
	City string `json:"city" jsonschema:"description=The city name to get the weather for"`
}

type Result string

func Handler(args Arguments) (Result, error) {
	slog.Info("query weather for city: " + args.City)

	url := fmt.Sprintf("https://wttr.in/%s?format=3", args.City)
	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to query weather, status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	result := string(body)
	slog.Info(result)

	return Result(result), nil
}
"#;

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
            fs::create_dir_all(output_dir.join("src")).await?;
            fs::write(output_dir.join("src/app.ts"), NODE_TEMPLATE_APP).await?;
            fs::write(output_dir.join("package.json"), NODE_TEMPLATE_PACKAGE_JSON).await?;
            fs::write(output_dir.join("tsconfig.json"), NODE_TEMPLATE_TSCONFIG).await?;
            info!("initialized node tool project: {:?}", output_dir);
            info!("next step: edit {:?}/src/app.ts", output_dir);
        }
        "go" => {
            fs::write(output_dir.join("app.go"), GO_TEMPLATE_APP).await?;
            info!("initialized go tool project: {:?}", output_dir);
            info!("next step: edit {:?}/app.go", output_dir);
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
