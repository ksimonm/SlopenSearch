//! SlopenSearch (Rust): configuration and wiring. The layers, from the
//! outside in: `http` (REST adapter) -> `search`, `aggs`, `suggest`,
//! `ingest`, `storage` -> `mapping`, `model` -> leaf libraries. See the
//! Layout section of README.md.

mod aggs;
mod analysis;
mod dates;
mod errors;
mod geohash;
mod hashes;
mod http;
mod ingest;
mod jregex;
mod jsonv;
mod lucene;
mod mapping;
mod model;
mod mustache;
mod painless;
mod resp;
mod scripting;
mod search;
mod settings;
mod snowball;
mod storage;
mod suggest;
mod uchar;

use std::sync::{Arc, Mutex};

fn main() {
    // Requests run on the blocking pool (http::handle); its threads get a large
    // stack once instead of a fresh big-stack thread per scripted request.
    let runtime = tokio::runtime::Builder::new_multi_thread()
        .enable_all()
        .thread_stack_size(painless::api::DEEP_STACK_SIZE)
        .build()
        .expect("tokio runtime");
    runtime.block_on(run());
}

async fn run() {
    let host = std::env::var("HOST").unwrap_or_else(|_| "127.0.0.1".to_string());
    let port = std::env::var("PORT").unwrap_or_else(|_| "9200".to_string());
    let addr = format!("{}:{}", host, port);
    let store: http::Shared = Arc::new(Mutex::new(storage::open()));
    let listener = match http::listen(&addr).await {
        Ok(l) => l,
        Err(e) => {
            eprintln!("slopensearch (rust): cannot listen on {}: {}", addr, e);
            std::process::exit(1);
        }
    };
    println!("slopensearch (rust) listening on {}", addr);
    if let Err(e) = http::serve(listener, store).await {
        eprintln!("slopensearch (rust): {}", e);
        std::process::exit(1);
    }
}
