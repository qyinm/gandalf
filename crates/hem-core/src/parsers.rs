use serde_json::{json, Value};

use crate::policy::{capture_status_for_key, is_secret_like_key, redact_structured_value};

#[derive(Debug, Clone, PartialEq)]
pub struct ParseSuccess {
    pub value: Value,
}

#[derive(Debug, Clone, PartialEq)]
pub struct ParseFailure {
    pub error: String,
}

#[derive(Debug, Clone, PartialEq)]
pub enum ParseResult {
    Ok(ParseSuccess),
    Err(ParseFailure),
}

#[derive(Debug, Clone, PartialEq)]
pub struct DotenvEntry {
    pub key: String,
    pub secret_like: bool,
    pub capture_status: &'static str,
}

pub fn parse_json(text: &str) -> ParseResult {
    match serde_json::from_str::<Value>(text) {
        Ok(value) => ParseResult::Ok(ParseSuccess {
            value: redact_structured_value(value),
        }),
        Err(error) => ParseResult::Err(ParseFailure {
            error: error.to_string(),
        }),
    }
}

pub fn parse_toml_key_values(text: &str) -> ParseResult {
    let mut value = serde_json::Map::new();
    let lines: Vec<&str> = text.split('\n').collect();
    let mut index = 0usize;

    while index < lines.len() {
        let line = lines[index].trim().trim_end_matches('\r').to_string();
        if line.is_empty() || line.starts_with('#') {
            index += 1;
            continue;
        }
        if line.starts_with('[') && line.ends_with(']') {
            index += 1;
            continue;
        }

        let Some((key, raw_value)) = parse_toml_key_value_line(&line) else {
            index += 1;
            continue;
        };

        let mut processed_value = raw_value.trim().to_string();

        if processed_value.starts_with('[') && !processed_value.ends_with(']') {
            let mut array_lines = vec![processed_value];
            index += 1;
            while index < lines.len() {
                let continuation_line = lines[index].trim().trim_end_matches('\r').to_string();
                let done = continuation_line.ends_with(']') || continuation_line.ends_with("],");
                array_lines.push(continuation_line);
                index += 1;
                if done {
                    break;
                }
            }
            processed_value = array_lines.join(" ");
        } else {
            index += 1;
        }

        let parsed = if is_secret_like_key(&key) {
            Value::String("[redacted]".to_string())
        } else {
            parse_toml_scalar(&processed_value)
        };
        value.insert(key, parsed);
    }

    ParseResult::Ok(ParseSuccess {
        value: Value::Object(value),
    })
}

pub fn parse_markdown(text: &str) -> ParseResult {
    let frontmatter = extract_markdown_frontmatter(text);
    let Some(frontmatter) = frontmatter else {
        return ParseResult::Ok(ParseSuccess {
            value: json!({ "hasFrontmatter": false }),
        });
    };

    let mut metadata = serde_json::Map::new();
    for raw_line in frontmatter.split('\n') {
        let line = raw_line.trim().trim_end_matches('\r');
        if let Some((key, raw_value)) = parse_markdown_metadata_line(line) {
            let parsed = if is_secret_like_key(&key) {
                Value::String("[redacted]".to_string())
            } else {
                Value::String(raw_value)
            };
            metadata.insert(key, parsed);
        }
    }

    ParseResult::Ok(ParseSuccess {
        value: json!({
            "hasFrontmatter": true,
            "metadata": Value::Object(metadata),
        }),
    })
}

pub fn parse_dotenv_keys(text: &str) -> Vec<DotenvEntry> {
    let mut entries = Vec::new();

    for raw_line in text.split('\n') {
        let line = raw_line.trim().trim_end_matches('\r');
        if line.is_empty() || line.starts_with('#') {
            continue;
        }

        let Some(key) = parse_dotenv_key(line) else {
            continue;
        };

        entries.push(DotenvEntry {
            key: key.clone(),
            secret_like: is_secret_like_key(&key),
            capture_status: capture_status_for_key(&key),
        });
    }

    entries
}

fn parse_toml_key_value_line(line: &str) -> Option<(String, String)> {
    let mut parts = line.splitn(2, '=');
    let key = parts.next()?.trim();
    let raw_value = parts.next()?;

    if key.is_empty() || !is_toml_key(key) {
        return None;
    }

    Some((key.to_string(), raw_value.to_string()))
}

fn is_toml_key(key: &str) -> bool {
    key.chars()
        .all(|ch| ch.is_ascii_alphanumeric() || matches!(ch, '_' | '.' | '-'))
}

fn parse_markdown_metadata_line(line: &str) -> Option<(String, String)> {
    let mut parts = line.splitn(2, ':');
    let key = parts.next()?.trim();
    let raw_value = parts.next()?.trim();

    if key.is_empty() || !is_toml_key(key) {
        return None;
    }

    Some((key.to_string(), raw_value.to_string()))
}

fn parse_dotenv_key(line: &str) -> Option<String> {
    let line = line.strip_prefix("export ").unwrap_or(line);
    let mut parts = line.splitn(2, '=');
    let key = parts.next()?.trim();

    if key.is_empty() {
        return None;
    }

    let valid = key
        .chars()
        .enumerate()
        .all(|(index, ch)| {
            if index == 0 {
                ch.is_ascii_alphabetic() || ch == '_'
            } else {
                ch.is_ascii_alphanumeric() || ch == '_'
            }
        });

    if valid {
        Some(key.to_string())
    } else {
        None
    }
}

fn extract_markdown_frontmatter(text: &str) -> Option<String> {
    let normalized = text.replace("\r\n", "\n");
    if !normalized.starts_with("---\n") {
        return None;
    }

    let rest = &normalized[4..];
    let end = rest.find("\n---")?;
    Some(rest[..end].to_string())
}

pub fn parse_toml_scalar(raw_value: &str) -> Value {
    let value = raw_value.trim().trim_end_matches(',');
    if (value.starts_with('"') && value.ends_with('"'))
        || (value.starts_with('\'') && value.ends_with('\''))
    {
        return Value::String(value[1..value.len() - 1].to_string());
    }
    if value == "true" {
        return Value::Bool(true);
    }
    if value == "false" {
        return Value::Bool(false);
    }
    if let Ok(number) = value.parse::<f64>() {
        if value.contains('.') {
            return json!(number);
        }
        if let Ok(integer) = value.parse::<i64>() {
            return json!(integer);
        }
        return json!(number);
    }
    if value.starts_with('[') && value.ends_with(']') {
        let inner = &value[1..value.len() - 1];
        let items: Vec<Value> = inner
            .split(',')
            .map(|entry| parse_toml_scalar(entry.trim()))
            .filter(|entry| !matches!(entry, Value::String(s) if s.is_empty()))
            .collect();
        return Value::Array(items);
    }
    Value::String(value.to_string())
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn parse_json_redacts_secret_keys() {
        let result = parse_json(r#"{"api_key":"secret","command":"npx"}"#);
        let ParseResult::Ok(ParseSuccess { value }) = result else {
            panic!("expected parse success");
        };
        assert_eq!(value["api_key"], "[redacted]");
        assert_eq!(value["command"], "npx");
    }

    #[test]
    fn parse_toml_handles_multiline_arrays() {
        let text = r#"args = [
  "--app",
  "desktop",
]"#;
        let ParseResult::Ok(ParseSuccess { value }) = parse_toml_key_values(text) else {
            panic!("expected parse success");
        };
        assert_eq!(value["args"], json!(["--app", "desktop"]));
    }

    #[test]
    fn parse_dotenv_keys_classifies_secret_like_keys() {
        let entries = parse_dotenv_keys("MODEL=gpt-5\nOPENAI_API_KEY=secret\n");
        assert_eq!(entries.len(), 2);
        assert_eq!(entries[0].capture_status, "omitted");
        assert_eq!(entries[1].capture_status, "redacted");
    }
}