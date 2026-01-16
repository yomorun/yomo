use anyhow::Result;
use clap::Parser;

use yomo::{
    sfn::Sfn,
    zipper::{Zipper, ZipperConfig},
};

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

    #[clap(short, long, default_value = "localhost:9000")]
    zipper: String,

    #[clap(short = 'd', long, default_value = "")]
    credential: String,
}

#[tokio::main]
async fn main() -> Result<()> {
    env_logger::init();

    let args = Cli::parse();

    match args {
        Cli::Serve(_) => {
            let zipper = Zipper::new("".to_string());
            zipper.serve(ZipperConfig::builder().build()).await
        }
        Cli::Run(opt) => {
            let sfn = Sfn::builder()
                .sfn_name(opt.name)
                .zipper(opt.zipper)
                .credential(opt.credential)
                .build();
            sfn.serve().await
        }
    }
}
