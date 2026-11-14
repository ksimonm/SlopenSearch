//! SlopenSearch (Rust): configuration and wiring. The layers, from the
//! outside in: `http` (REST adapter) -> `search`, `aggs`, `suggest`,
//! `ingest`, `storage` -> `mapping`, `model` -> leaf libraries. See the
//! Layout section of README.md.

mod aggs;
mod analysis;
mod dates;
mod errors;
mod fields;
mod hashes;
mod hdr;
mod http;
mod ingest;
mod ingest_script;
mod jregex;
mod jsonv;
mod knn;
mod knn_index;
mod lucene;
mod mapping;
mod model;
mod mustache;
mod painless;
mod persist;
mod resp;
mod search;
mod settings;
mod snowball;
mod suggest;
mod suggest_completion;
mod suggest_phrase;
mod uchar;

use std::sync::{Arc, Mutex};

#[tokio::main]
async fn main() {
    let host = std::env::var("HOST").unwrap_or_else(|_| "127.0.0.1".to_string());
    let port = std::env::var("PORT").unwrap_or_else(|_| "9200".to_string());
    let addr = format!("{}:{}", host, port);
    let store: http::Shared = Arc::new(Mutex::new(persist::open()));
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
