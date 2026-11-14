//! Mustache templates with mustache.java semantics (search templates, phrase
//! suggester collate queries).

use serde_json::Value;

#[derive(Debug, Clone)]
pub struct MustacheError {
    pub tag: String,
}

#[derive(Debug, Clone)]
enum Node {
    Text(String),
    Var { name: String, escaped: bool },
    Section {
        name: String,
        params: String,
        inverted: bool,
        children: Vec<Node>,
    },
}

fn first_word(s: &str) -> &str {
    s.split_whitespace().next().unwrap_or("")
}

fn parse_template(src: &str) -> Result<Vec<Node>, MustacheError> {
    let mut root: Vec<Node> = Vec::new();
    let mut stack: Vec<(String, String, bool, Vec<Node>)> = Vec::new();
    let mut open = "{{".to_string();
    let mut close = "}}".to_string();
    let mut pos = 0usize;
    let bytes = src.as_bytes();
    while pos < src.len() {
        let Some(rel) = src[pos..].find(open.as_str()) else {
            let text = &src[pos..];
            if !text.is_empty() {
                push_node(&mut root, &mut stack, Node::Text(text.to_string()));
            }
            break;
        };
        let start = pos + rel;
        if start > pos {
            push_node(
                &mut root,
                &mut stack,
                Node::Text(src[pos..start].to_string()),
            );
        }
        if open == "{{" && bytes.get(start + 2) == Some(&b'{') {
            let body_start = start + 3;
            match src[body_start..].find("}}}") {
                Some(end_rel) => {
                    let name = src[body_start..body_start + end_rel].trim().to_string();
                    push_node(&mut root, &mut stack, Node::Var { name, escaped: false });
                    pos = body_start + end_rel + 3;
                    continue;
                }
                None => {
                    return Err(MustacheError {
                        tag: String::new(),
                    })
                }
            }
        }
        let body_start = start + open.len();
        let Some(end_rel) = src[body_start..].find(close.as_str()) else {
            let text = &src[start..];
            push_node(&mut root, &mut stack, Node::Text(text.to_string()));
            break;
        };
        let tag = src[body_start..body_start + end_rel].trim().to_string();
        let _ = (&open, &close);
        pos = body_start + end_rel + close.len();
        if tag.starts_with('=') && tag.ends_with('=') {
            let inner = tag.trim_matches('=').trim();
            let parts: Vec<&str> = inner.split_whitespace().collect();
            if parts.len() == 2 {
                open = parts[0].to_string();
                close = parts[1].to_string();
            }
            continue;
        }
        if tag.starts_with('!') {
            continue;
        }
        if let Some(rest) = tag.strip_prefix('#') {
            let name = first_word(rest).to_string();
            let params = rest[name.len()..].trim().to_string();
            stack.push((name, params, false, Vec::new()));
            continue;
        }
        if let Some(rest) = tag.strip_prefix('^') {
            let name = first_word(rest).to_string();
            let params = rest[name.len()..].trim().to_string();
            stack.push((name, params, true, Vec::new()));
            continue;
        }
        if let Some(rest) = tag.strip_prefix('/') {
            let name = first_word(rest);
            if let Some((n, params, inverted, children)) = stack.pop() {
                if n != name {
                    return Err(MustacheError { tag: n });
                }
                let node = Node::Section {
                    name: n,
                    params,
                    inverted,
                    children,
                };
                push_node(&mut root, &mut stack, node);
            }
            continue;
        }
        if let Some(rest) = tag.strip_prefix('&') {
            push_node(
                &mut root,
                &mut stack,
                Node::Var {
                    name: rest.trim().to_string(),
                    escaped: false,
                },
            );
            continue;
        }
        if tag.starts_with('>') {
            continue;
        }
        push_node(
            &mut root,
            &mut stack,
            Node::Var {
                name: tag,
                escaped: true,
            },
        );
    }
    if let Some((name, _, _, _)) = stack.last() {
        return Err(MustacheError { tag: name.clone() });
    }
    Ok(root)
}

fn push_node(root: &mut Vec<Node>, stack: &mut [(String, String, bool, Vec<Node>)], node: Node) {
    if let Some(top) = stack.last_mut() {
        top.3.push(node);
    } else {
        root.push(node);
    }
}

fn descend<'a>(val: &'a Value, path: &str) -> Option<&'a Value> {
    let mut cur = val;
    for seg in path.split('.') {
        match cur {
            Value::Object(m) => cur = m.get(seg)?,
            Value::Array(a) => cur = a.get(seg.parse::<usize>().ok()?)?,
            _ => return None,
        }
    }
    Some(cur)
}

fn resolve<'a>(ctx: &'a [Value], name: &str) -> Option<&'a Value> {
    if name == "." {
        return ctx.last();
    }
    for frame in ctx.iter().rev() {
        if let Some(v) = descend(frame, name) {
            return Some(v);
        }
    }
    None
}

fn json_escape(s: &str) -> String {
    let quoted = serde_json::to_string(s).unwrap_or_else(|_| "\"\"".to_string());
    quoted[1..quoted.len() - 1].to_string()
}

fn java_scalar(v: &Value) -> String {
    match v {
        Value::Null => "null".to_string(),
        Value::Bool(b) => b.to_string(),
        Value::Number(n) => n.to_string(),
        Value::String(s) => s.clone(),
        Value::Array(a) => {
            let parts: Vec<String> = a
                .iter()
                .enumerate()
                .map(|(i, x)| format!("{}={}", i, java_scalar(x)))
                .collect();
            format!("{{{}}}", parts.join(", "))
        }
        Value::Object(m) => {
            let parts: Vec<String> = m
                .iter()
                .map(|(k, x)| format!("{}={}", k, java_scalar(x)))
                .collect();
            format!("{{{}}}", parts.join(", "))
        }
    }
}

fn interpolation(v: Option<&Value>, escaped: bool) -> String {
    let s = match v {
        None | Some(Value::Null) => String::new(),
        Some(Value::String(s)) => s.clone(),
        Some(other) => java_scalar(other),
    };
    if escaped {
        json_escape(&s)
    } else {
        s
    }
}

fn is_truthy(v: Option<&Value>) -> bool {
    match v {
        None | Some(Value::Null) => false,
        Some(Value::Bool(b)) => *b,
        Some(Value::String(s)) => !s.is_empty(),
        Some(Value::Array(a)) => !a.is_empty(),
        Some(_) => true,
    }
}

fn java_url_encode(s: &str) -> String {
    let mut out = String::new();
    for b in s.bytes() {
        match b {
            b'A'..=b'Z' | b'a'..=b'z' | b'0'..=b'9' | b'-' | b'_' | b'.' | b'*' => {
                out.push(b as char)
            }
            b' ' => out.push('+'),
            _ => out.push_str(&format!("%{:02X}", b)),
        }
    }
    out
}

fn raw_text(children: &[Node]) -> String {
    let mut out = String::new();
    for n in children {
        if let Node::Text(t) = n {
            out.push_str(t);
        }
    }
    out
}

fn render_nodes(nodes: &[Node], ctx: &mut Vec<Value>, out: &mut String) {
    for node in nodes {
        match node {
            Node::Text(t) => out.push_str(t),
            Node::Var { name, escaped } => {
                let v = resolve(ctx, name);
                out.push_str(&interpolation(v, *escaped));
            }
            Node::Section {
                name,
                params,
                inverted,
                children,
            } => {
                if name == "toJson" || name == "join" || name == "url" {
                    let body = raw_text(children).trim().to_string();
                    let mut rendered_body = String::new();
                    if body.is_empty() {
                        render_nodes(children, ctx, &mut rendered_body);
                    }
                    let resolved = resolve(ctx, &body);
                    let direct = rendered_body.trim().to_string();
                    match name.as_str() {
                        "toJson" => {
                            if let Some(val) = resolved {
                                if !val.is_null() {
                                    out.push_str(&serde_json::to_string(val).unwrap_or_default());
                                }
                            }
                        }
                        "join" => {
                            let delim = params
                                .split("delimiter=")
                                .nth(1)
                                .map(|s| s.trim().trim_matches('\'').trim_matches('"').to_string())
                                .unwrap_or_else(|| ",".to_string());
                            if let Some(Value::Array(a)) = resolved {
                                let parts: Vec<String> = a.iter().map(java_scalar).collect();
                                out.push_str(&parts.join(&delim));
                            }
                        }
                        "url" => {
                            if !direct.is_empty() {
                                out.push_str(&java_url_encode(&direct));
                            } else if let Some(val) = resolved {
                                out.push_str(&java_url_encode(&java_scalar(val)));
                            }
                        }
                        _ => {}
                    }
                    continue;
                }
                let v = resolve(ctx, name);
                if *inverted {
                    if !is_truthy(v) {
                        render_nodes(children, ctx, out);
                    }
                } else if let Some(Value::Array(a)) = v {
                    for item in a.clone() {
                        ctx.push(item);
                        render_nodes(children, ctx, out);
                        ctx.pop();
                    }
                } else if is_truthy(v) {
                    ctx.push(v.cloned().unwrap_or(Value::Null));
                    render_nodes(children, ctx, out);
                    ctx.pop();
                }
            }
        }
    }
}

pub fn render_mustache(src: &str, params: &Value) -> Result<String, MustacheError> {
    let nodes = parse_template(src)?;
    let mut ctx = vec![params.clone()];
    let mut out = String::new();
    render_nodes(&nodes, &mut ctx, &mut out);
    Ok(out)
}

pub(crate) fn render_source(src: &Value, params: &Value) -> Result<String, MustacheError> {
    let text = match src {
        Value::String(s) => s.clone(),
        other => serde_json::to_string(other).unwrap_or_default(),
    };
    render_mustache(&text, params)
}
