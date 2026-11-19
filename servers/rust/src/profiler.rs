//! In-process sampling profiler (pprof-rs: SIGPROF timer, no perf or root needed).
//!
//!   POST /_slop/profile/start?frequency=999   start sampling every thread
//!   POST /_slop/profile/stop                  stop; flamegraph SVG
//!   POST /_slop/profile/stop?format=folded    stop; folded stacks (inferno, speedscope)
//!
//! Nothing is sampled until a profile is started. `scripts/profile.sh` drives it.

use std::sync::Mutex;

use crate::errors::text_response;
use crate::http::Req;
use axum::response::Response;

static GUARD: Mutex<Option<pprof::ProfilerGuard<'static>>> = Mutex::new(None);

pub fn handle(req: &Req) -> Response {
    let action = req.path.trim_start_matches("/_slop/profile").trim_matches('/');
    let mut guard = GUARD.lock().unwrap_or_else(|e| e.into_inner());
    match action {
        "start" => {
            if guard.is_some() {
                return text_response(409, "text/plain", "a profile is running\n".into());
            }
            let freq = req.param("frequency").and_then(|f| f.parse().ok()).unwrap_or(999);
            let g = pprof::ProfilerGuardBuilder::default()
                .frequency(freq)
                .blocklist(&["libc", "libgcc", "pthread", "vdso"])
                .build();
            match g {
                Ok(g) => {
                    *guard = Some(g);
                    text_response(200, "text/plain", format!("profiling at {} Hz\n", freq))
                }
                Err(e) => text_response(500, "text/plain", format!("{}\n", e)),
            }
        }
        "stop" => {
            let Some(g) = guard.take() else {
                return text_response(409, "text/plain", "no profile is running\n".into());
            };
            let report = match g.report().build() {
                Ok(r) => r,
                Err(e) => return text_response(500, "text/plain", format!("{}\n", e)),
            };
            if req.param("format") == Some("folded") {
                let mut out = String::new();
                for (frames, count) in &report.data {
                    let mut names: Vec<String> = Vec::new();
                    for f in frames.frames.iter().rev() {
                        for s in f.iter().rev() {
                            names.push(s.name());
                        }
                    }
                    out.push_str(&frames.thread_name);
                    for n in names {
                        out.push(';');
                        out.push_str(&n);
                    }
                    out.push_str(&format!(" {}\n", count));
                }
                return text_response(200, "text/plain", out);
            }
            let mut svg = Vec::new();
            match report.flamegraph(&mut svg) {
                Ok(()) => text_response(200, "image/svg+xml", String::from_utf8_lossy(&svg).into_owned()),
                Err(e) => text_response(500, "text/plain", format!("{}\n", e)),
            }
        }
        _ => text_response(404, "text/plain", "use /_slop/profile/start or /_slop/profile/stop\n".into()),
    }
}
