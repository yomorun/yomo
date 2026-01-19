use anyhow::Result;
use clap::Parser;
use config::{Config, File};
use log::{debug, info};

use serde::Deserialize;
use tokio::select;
use yomo::{
    bridge::http::{
        middleware::HttpMiddlewareImpl,
        server::{HttpBridgeConfig, serve_http_bridge},
    },
    sfn::client::Sfn,
    tls::TlsConfig,
    zipper::{
        middleware::{ZipperMiddlewareImpl, ZipperMiddlewareImplConfig},
        server::{Zipper, ZipperConfig},
    },
};

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
    #[clap(short, long, help = "yomo Serverless LLM Function name")]
    name: String,

    #[clap(
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

#[derive(Debug, Clone, Deserialize, Default)]
struct ServeConfig {
    #[serde(default)]
    zipper: ZipperConfig,

    #[serde(default)]
    zipper_middleware: ZipperMiddlewareImplConfig,

    #[serde(default)]
    http_bridge: HttpBridgeConfig,
}

async fn serve(opt: ServeOptions) -> Result<()> {
    let config = match opt.config {
        Some(file) => {
            debug!("using config file: {}", file);

            Config::builder()
                .add_source(File::with_name(&file))
                .build()?
                .try_deserialize::<ServeConfig>()?
        }
        None => {
            debug!("using default config");

            ServeConfig::default()
        }
    };

    info!("config: {:?}", config);

    let zipper_middleware = ZipperMiddlewareImpl::new(config.zipper_middleware);
    let zipper = Zipper::new(zipper_middleware);

    select! {
        r = serve_http_bridge(
            &config.http_bridge,
            zipper.clone(),
            HttpMiddlewareImpl::default(),
        ) => r,
        r = zipper.serve(config.zipper) => r,
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

    let sfn = Sfn::builder().sfn_name(opt.name).build();

    sfn.run(&opt.zipper, &opt.credential, &tls_config, opt.tls_insecure)
        .await
}

#[tokio::main]
async fn main() -> Result<()> {
    env_logger::init();

    match Cli::parse() {
        Cli::Serve(opt) => serve(opt).await,
        Cli::Run(opt) => run(opt).await,
    }
}
