//! HTTP body compression: request bodies with a gzip/deflate Content-Encoding
//! are inflated, and responses are compressed the way Netty's
//! HttpContentCompressor negotiates Accept-Encoding (gzip, deflate and framed
//! snappy; brotli and zstd are not available in OpenSearch's transport).

use std::io::{Read, Write};

use axum::http::HeaderMap;
use axum::response::Response;

use crate::errors::ApiError;

/// OpenSearch's default `http.compression_level`.
const LEVEL: u32 = 3;

/// Undoes a gzip or deflate (zlib) Content-Encoding; other codings pass
/// through unchanged. `limit` caps the inflated size (one byte over it is kept
/// so the caller's own size check still fires).
pub fn decode_request_body(headers: &HeaderMap, body: Vec<u8>, limit: usize) -> Result<Vec<u8>, ApiError> {
    let enc = headers
        .get("content-encoding")
        .and_then(|v| v.to_str().ok())
        .unwrap_or("")
        .trim()
        .to_ascii_lowercase();
    let reader: Box<dyn Read> = match enc.as_str() {
        "gzip" | "x-gzip" => Box::new(flate2::read::MultiGzDecoder::new(&body[..])),
        "deflate" => Box::new(flate2::read::ZlibDecoder::new(&body[..])),
        _ => return Ok(body),
    };
    let mut plain = Vec::new();
    reader
        .take(limit as u64 + 1)
        .read_to_end(&mut plain)
        .map_err(|_| ApiError::bad("failed to decompress request body"))?;
    Ok(plain)
}

/// Picks the response Content-Encoding for an Accept-Encoding header value.
pub fn response_encoding(accept: &str) -> Option<&'static str> {
    if accept.is_empty() {
        return None;
    }
    let (mut gz, mut df, mut sn, mut star) = (-1.0f64, -1.0f64, -1.0f64, -1.0f64);
    for tok in accept.split(',') {
        let (name, params) = tok.split_once(';').unwrap_or((tok, ""));
        let q = match params.find("q=") {
            Some(i) => params[i + 2..].trim().parse::<f64>().unwrap_or(0.0),
            None => 1.0,
        };
        match name.trim().to_ascii_lowercase().as_str() {
            "gzip" | "x-gzip" => gz = q,
            "deflate" => df = q,
            "snappy" => sn = q,
            "*" => star = q,
            _ => {}
        }
    }
    if sn > 0.0 && sn >= gz && sn >= df {
        return Some("snappy");
    }
    if gz > 0.0 && gz >= df {
        return Some("gzip");
    }
    if df > 0.0 {
        return Some("deflate");
    }
    if star > 0.0 {
        if sn == -1.0 {
            return Some("snappy");
        }
        if gz == -1.0 {
            return Some("gzip");
        }
        if df == -1.0 {
            return Some("deflate");
        }
    }
    None
}

fn compress(enc: &str, body: &[u8]) -> Vec<u8> {
    let level = flate2::Compression::new(LEVEL);
    let out = match enc {
        "gzip" => {
            let mut w = flate2::write::GzEncoder::new(Vec::new(), level);
            w.write_all(body).and_then(|_| w.finish())
        }
        "deflate" => {
            let mut w = flate2::write::ZlibEncoder::new(Vec::new(), level);
            w.write_all(body).and_then(|_| w.finish())
        }
        "snappy" => {
            let mut w = snap::write::FrameEncoder::new(Vec::new());
            w.write_all(body)
                .and_then(|_| w.into_inner().map_err(|e| e.into_error()))
        }
        _ => Ok(body.to_vec()),
    };
    out.unwrap_or_else(|_| body.to_vec())
}

/// Compresses a non-empty response body when the request accepts an encoding
/// and sets Content-Encoding and the compressed Content-Length.
pub async fn encode_response(accept: &str, resp: Response) -> Response {
    let Some(enc) = response_encoding(accept) else {
        return resp;
    };
    if resp.headers().contains_key("content-encoding") {
        return resp;
    }
    let (mut parts, body) = resp.into_parts();
    let bytes = axum::body::to_bytes(body, usize::MAX).await.unwrap_or_default();
    if bytes.is_empty() {
        return Response::from_parts(parts, axum::body::Body::from(bytes));
    }
    let out = compress(enc, &bytes);
    parts.headers.insert("content-encoding", enc.parse().unwrap());
    parts.headers.insert("content-length", out.len().into());
    Response::from_parts(parts, axum::body::Body::from(out))
}
