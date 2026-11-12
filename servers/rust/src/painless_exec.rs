// The _scripts/painless/_execute API; the engine lives in pl_*.rs.

use crate::{Req, Store};
use axum::response::Response;

pub fn handle_execute(store: &mut Store, req: &Req, _lang: &str) -> Response {
    if req.method != "POST" && req.method != "GET" {
        return crate::errors::method_not_allowed(req, &["POST"]);
    }
    crate::pl_api::run_with_stack(|| crate::pl_api::handle_execute(store, req))
}
