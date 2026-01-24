use std::process;

use anyhow::Result;
use clap::Parser;
use config::{Config, File};
use log::{error, info};

use serde::Deserialize;
use tokio::{select, sync::mpsc};
use yomo::{
    bridge::Bridge,
    connector::MemoryConnector,
    http::serve_http,
    sfn::{client::Sfn, serverless::ServerlessHandler},
    tls::TlsConfig,
    zipper::{router::RouterImpl, server::Zipper},
};

fn default_host() -> String {
    "127.0.0.1".to_string()
}

fn default_quic_port() -> u16 {
    9000
}

fn default_http_port() -> u16 {
    9001
}

#[derive(Parser, Debug)]
#[command(author, version, about, long_about = None)]
enum Cli {
    /// Serve a YoMo Service (Zipper)
    Serve(ServeOptions),

    /// Run a YoMo Serverless LLM Function
    Run(RunOptions),
}

#[derive(Parser, Debug)]
struct ServeOptions {
    #[clap(short, long, help = "path to the YoMo server configuration file")]
    config: Option<String>,
}

#[derive(Parser, Debug)]
struct RunOptions {
    #[clap(
        default_value = ".",
        help = "directory to the serverless function source file"
    )]
    serverless_dir: String,

    #[clap(short, long, help = "yomo Serverless LLM Function name")]
    name: String,

    #[clap(
        short,
        long,
        default_value = "localhost:9000",
        help = "YoMo-Zipper endpoint addr"
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
        long,
        default_value_t = false,
        help = "enable the insecure mode will skip server name verification"
    )]
    tls_insecure: bool,
}

#[derive(Debug, Clone, Deserialize)]
struct ServeConfig {
    #[serde(default = "default_host")]
    host: String,

    #[serde(default = "default_quic_port")]
    quic_port: u16,

    #[serde(default = "default_http_port")]
    http_port: u16,

    #[serde(default)]
    tls: TlsConfig,

    #[serde(default)]
    auth_token: Option<String>,
}

impl Default for ServeConfig {
    fn default() -> Self {
        Self {
            host: default_host(),
            quic_port: default_quic_port(),
            http_port: default_http_port(),
            tls: TlsConfig::default(),
            auth_token: None,
        }
    }
}

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
    let connector = MemoryConnector::new(sender);

    let router = RouterImpl::new(config.auth_token);
    let zipper = Zipper::new(router, receiver);

    select! {
        r = serve_http(&config.host, config.http_port, connector) => r,
        r = zipper.clone().serve_bridge() => r,
        r = zipper.listen_for_quic(&config.host, config.quic_port, &config.tls) => r,
    }?;

    Ok(())
}

async fn run(opt: RunOptions) -> Result<()> {
    let tls_config = TlsConfig::builder()
        .maybe_ca_cert(opt.tls_ca_cert_file)
        .maybe_cert(opt.tls_cert_file)
        .maybe_key(opt.tls_key_file)
        .mutual(opt.tls_mutual)
        .build();

    let serverless_handler = ServerlessHandler::default();

    let mut sfn = Sfn::new(opt.name, serverless_handler.clone());
    sfn.connect_zipper(&opt.zipper, &opt.credential, &tls_config, opt.tls_insecure)
        .await?;

    select! {
        r = serverless_handler.run_subprocess(&opt.serverless_dir) => r,
        r = sfn.serve_bridge() => r,
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
