use std::sync::Arc;

use anyhow::Result;
use clap::Parser;
use config::{Config, File};
use log::{debug, info};

use serde::Deserialize;
use tokio::select;
use yomo::{
    bridge::http::{
        config::HttpBridgeConfig, middleware::HttpMiddlewareImpl, server::serve_http_bridge,
    },
    sfn::{client::Sfn, handler::HandlerImpl},
    tls::TlsConfig,
    zipper::{
        config::{ZipperConfig, ZipperMiddlewareImplConfig},
        middleware::ZipperMiddlewareImpl,
        server::Zipper,
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

    #[clap(long, help = "client credential payload")]
    credential: Option<String>,

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
    pub zipper: ZipperConfig,

    #[serde(default)]
    pub zipper_middleware: ZipperMiddlewareImplConfig,

    #[serde(default)]
    pub http_bridge: HttpBridgeConfig,
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
    let zipper = Zipper::new(config.zipper, zipper_middleware);

    select! {
        r = serve_http_bridge(
            &config.http_bridge,
            Arc::new(HttpMiddlewareImpl::default()),
            Arc::new(zipper.clone()),
        ) => r,
        r = zipper.serve() => r,
    }?;

    Ok(())
}

async fn run(opt: RunOptions) -> Result<()> {
    Sfn::builder()
        .sfn_name(opt.name)
        .zipper(opt.zipper)
        .maybe_credential(opt.credential)
        .tls_config(TlsConfig {
            ca_cert: opt.tls_ca_cert_file,
            cert: opt.tls_cert_file,
            key: opt.tls_key_file,
            mutual: opt.tls_mutual,
        })
        .tls_insecure(opt.tls_insecure)
        .handler(Arc::new(HandlerImpl::default()))
        .build()
        .serve()
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
