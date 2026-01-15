use anyhow::Result;
use clap::Parser;

use yomo::{sfn::Sfn, zipper::Zipper};

#[derive(Parser, Debug)]
#[command(author, version, about, long_about = None)]
enum Cli {
    Serve(ServeOptions),

    Run(RunOptions),
}

#[derive(Parser, Debug)]
struct ServeOptions {}

#[derive(Parser, Debug)]
struct RunOptions {
    #[clap(short, long)]
    name: String,
}

#[tokio::main]
async fn main() -> Result<()> {
    env_logger::init();

    let args = Cli::parse();

    match args {
        Cli::Serve(_) => {
            let server = Zipper::builder().build();
            server.serve().await
        }
        Cli::Run(options) => {
            let sfn = Sfn::builder().sfn_name(options.name).build();
            sfn.serve().await
        }
    }
}
