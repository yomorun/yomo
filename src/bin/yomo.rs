use anyhow::Result;
use clap::Parser;
use config::{Config, File};
use log::{debug, info};

use yomo::{
    config::ServeConfig,
    sfn::Sfn,
    tls::TlsConfig,
    zipper::{config::MiddlewareConfig, middleware::DefaultMiddleware, server::Zipper},
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

#[tokio::main]
async fn main() -> Result<()> {
    env_logger::init();

    match Cli::parse() {
        Cli::Serve(opt) => {
            let c = match opt.config {
                Some(file) => {
                    debug!("using config file: {}", file);

                    Config::builder()
                        .add_source(File::with_name(&file))
                        .build()?
                        .try_deserialize::<ServeConfig<MiddlewareConfig>>()?
                }
                None => {
                    debug!("using default config");

                    ServeConfig::default()
                }
            };

            info!("config: {:?}", c);

            Zipper::new(DefaultMiddleware::new(c.zipper.middleware))
                .serve(&c.zipper.quic, &c.zipper.http)
                .await
        }
        Cli::Run(opt) => {
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
                .build()
                .serve()
                .await
        }
    }
}
