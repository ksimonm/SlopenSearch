//! Mustache templates with mustache.java semantics (search templates, phrase
//! suggester collate queries, ingest processor templates).

use serde_json::Value;

/// A mustache.java compile error, or a value lookup that failed while rendering;
/// `line` is the 1-based template line it names (newlines inside tags are not
/// counted, as in mustache.java).
#[derive(Debug, Clone)]
pub struct MustacheError {
    pub kind: MustacheErrorKind,
    pub line: usize,
}

#[derive(Debug, Clone)]
pub enum MustacheErrorKind {
    /// `{{` without its closing delimiter
    UnclosedVariable,
    /// `{{{name}}` without the third brace; `col` is the position in the line
    UnclosedTripleVariable { name: String, col: usize },
    /// a section opened with `{{#name}}` / `{{^name}}` and never closed
    UnclosedSection(String),
    /// `{{/close}}` that does not close the innermost open section
    Mismatched { open: Option<String>, close: String },
    /// the template ends right after `{{`
    Empty,
    /// `{{>name}}`
    Partial(String),
    /// rendering: a list lookup out of range (`name` is the whole tag); `cause` is the
    /// message of the intermediate exception, `java` the Java exception (type, message)
    Lookup { name: String, cause: String, java: (String, String) },
}

impl MustacheError {
    /// The mustache.java message for a template named `file`.
    pub fn describe(&self, file: &str) -> String {
        let line = self.line;
        let msg = match &self.kind {
            MustacheErrorKind::UnclosedVariable => "Improperly closed variable".to_string(),
            MustacheErrorKind::UnclosedTripleVariable { name, col } => {
                format!("Improperly closed variable: {name} in {file}:{line}@{col}")
            }
            MustacheErrorKind::UnclosedSection(tag) => format!("Failed to close '{tag}' tag"),
            MustacheErrorKind::Mismatched { open, close } => format!(
                "Mismatched start/end tags: {} != {close} in {file}:{line}",
                open.as_deref().unwrap_or("null")
            ),
            MustacheErrorKind::Empty => "Empty mustache".to_string(),
            MustacheErrorKind::Partial(name) => {
                return format!("Partial templates are not supported in OpenSearch Mustache scripts: [{name}]")
            }
            MustacheErrorKind::Lookup { name, .. } => format!("Failed to get value for {name}"),
        };
        format!("{msg} @[{file}:{line}]")
    }

    /// For a failed lookup: the message of its cause and the Java exception under it.
    pub fn lookup_cause(&self, file: &str) -> Option<(String, &str, &str)> {
        match &self.kind {
            MustacheErrorKind::Lookup { cause, java, .. } => {
                Some((format!("{cause} @[{file}:{}]", self.line), java.0.as_str(), java.1.as_str()))
            }
            _ => None,
        }
    }
}

#[derive(Debug, Clone)]
enum Node {
    Text(String),
    Var { name: String, escaped: bool, line: usize },
    Section {
        line: usize,
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
    // open sections: name, parameters, inverted, children, line of the opening tag
    let mut stack: Vec<(String, String, bool, Vec<Node>, usize)> = Vec::new();
    let mut line = 1usize;
    let err = |kind, line| Err(MustacheError { kind, line });
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
        line += src[pos..start].matches('\n').count();
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
                    push_node(&mut root, &mut stack, Node::Var { name, escaped: false, line });
                    pos = body_start + end_rel + 3;
                    continue;
                }
                None => {
                    let line_start = src[..start].rfind('\n').map_or(0, |i| i + 1);
                    let (name, end) = match src[body_start..].find("}}") {
                        Some(e) => (&src[body_start..body_start + e], body_start + e + 2),
                        None => (&src[body_start..], src.len().saturating_sub(1)),
                    };
                    let col = src[line_start..end].chars().count();
                    let name = name.trim().to_string();
                    return err(MustacheErrorKind::UnclosedTripleVariable { name, col }, line);
                }
            }
        }
        let body_start = start + open.len();
        if body_start >= src.len() {
            return err(MustacheErrorKind::Empty, line);
        }
        let Some(end_rel) = src[body_start..].find(close.as_str()) else {
            let rest = src[body_start..].trim_start();
            if let Some(section) = rest.strip_prefix('#').or_else(|| rest.strip_prefix('^')) {
                return err(MustacheErrorKind::UnclosedSection(first_word(section).to_string()), line);
            }
            return err(MustacheErrorKind::UnclosedVariable, line);
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
            stack.push((name, params, false, Vec::new(), line));
            continue;
        }
        if let Some(rest) = tag.strip_prefix('^') {
            let name = first_word(rest).to_string();
            let params = rest[name.len()..].trim().to_string();
            stack.push((name, params, true, Vec::new(), line));
            continue;
        }
        if let Some(rest) = tag.strip_prefix('/') {
            let name = first_word(rest);
            let Some((n, params, inverted, children, open_line)) = stack.pop() else {
                let close = name.to_string();
                return err(MustacheErrorKind::Mismatched { open: None, close }, line);
            };
            if n != name {
                let close = name.to_string();
                return err(MustacheErrorKind::Mismatched { open: Some(n), close }, line);
            }
            let node = Node::Section {
                line: open_line,
                name: n,
                params,
                inverted,
                children,
            };
            push_node(&mut root, &mut stack, node);
            continue;
        }
        if let Some(rest) = tag.strip_prefix('&') {
            push_node(
                &mut root,
                &mut stack,
                Node::Var {
                    name: rest.trim().to_string(),
                    escaped: false,
                    line,
                },
            );
            continue;
        }
        if let Some(rest) = tag.strip_prefix('>') {
            return err(MustacheErrorKind::Partial(rest.trim().to_string()), line);
        }
        push_node(
            &mut root,
            &mut stack,
            Node::Var {
                name: tag,
                escaped: true,
                line,
            },
        );
    }
    if let Some((name, _, _, _, open_line)) = stack.last() {
        return err(MustacheErrorKind::UnclosedSection(name.clone()), *open_line);
    }
    Ok(root)
}

fn push_node(root: &mut Vec<Node>, stack: &mut [(String, String, bool, Vec<Node>, usize)], node: Node) {
    if let Some(top) = stack.last_mut() {
        top.3.push(node);
    } else {
        root.push(node);
    }
}

type Lookup<'a> = Result<Option<&'a Value>, MustacheError>;

/// Lists are indexed like OpenSearch's list wrapper: a numeric segment past the end
/// (or negative) fails the lookup, any other segment finds nothing.
fn descend<'a>(val: &'a Value, name: &str, line: usize) -> Lookup<'a> {
    let fail = |cause: String, typ: &str, msg: String| MustacheError {
        kind: MustacheErrorKind::Lookup { name: name.to_string(), cause, java: (typ.to_string(), msg) },
        line,
    };
    let mut cur = val;
    for seg in name.split('.') {
        match cur {
            Value::Object(m) => match m.get(seg) {
                Some(v) => cur = v,
                None => return Ok(None),
            },
            Value::Array(a) => match seg.parse::<i64>() {
                Ok(i) if i < 0 => {
                    return Err(fail("position >= 0".into(), "illegal_argument_exception", "position >= 0".into()))
                }
                Ok(i) => match a.get(i as usize) {
                    Some(v) => cur = v,
                    None => return Err(fail(i.to_string(), "index_out_of_bounds_exception", i.to_string())),
                },
                Err(_) => return Ok(None),
            },
            _ => return Ok(None),
        }
    }
    Ok(Some(cur))
}

fn resolve<'a>(ctx: &'a [Value], name: &str, line: usize) -> Lookup<'a> {
    if name == "." {
        return Ok(ctx.last());
    }
    for frame in ctx.iter().rev() {
        if let Some(v) = descend(frame, name, line)? {
            return Ok(Some(v));
        }
    }
    Ok(None)
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

fn render_nodes(nodes: &[Node], ctx: &mut Vec<Value>, out: &mut String) -> Result<(), MustacheError> {
    for node in nodes {
        match node {
            Node::Text(t) => out.push_str(t),
            Node::Var { name, escaped, line } => {
                let v = resolve(ctx, name, *line)?;
                out.push_str(&interpolation(v, *escaped));
            }
            Node::Section {
                line,
                name,
                params,
                inverted,
                children,
            } => {
                if name == "toJson" || name == "join" || name == "url" {
                    let body = raw_text(children).trim().to_string();
                    let mut rendered_body = String::new();
                    if body.is_empty() {
                        render_nodes(children, ctx, &mut rendered_body)?;
                    }
                    let resolved = resolve(ctx, &body, *line)?;
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
                let v = resolve(ctx, name, *line)?;
                if *inverted {
                    if !is_truthy(v) {
                        render_nodes(children, ctx, out)?;
                    }
                } else if let Some(Value::Array(a)) = v {
                    for item in a.clone() {
                        ctx.push(item);
                        let r = render_nodes(children, ctx, out);
                        ctx.pop();
                        r?;
                    }
                } else if is_truthy(v) {
                    ctx.push(v.cloned().unwrap_or(Value::Null));
                    let r = render_nodes(children, ctx, out);
                    ctx.pop();
                    r?;
                }
            }
        }
    }
    Ok(())
}

pub fn render_mustache(src: &str, params: &Value) -> Result<String, MustacheError> {
    let nodes = parse_template(src)?;
    let mut ctx = vec![params.clone()];
    let mut out = String::new();
    render_nodes(&nodes, &mut ctx, &mut out)?;
    Ok(out)
}

pub(crate) fn render_source(src: &Value, params: &Value) -> Result<String, MustacheError> {
    let text = match src {
        Value::String(s) => s.clone(),
        other => serde_json::to_string(other).unwrap_or_default(),
    };
    render_mustache(&text, params)
}
