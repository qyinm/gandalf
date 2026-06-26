package parsers

import (
	"reflect"
	"testing"
)

func TestParseJSONRedactsSecretKeys(t *testing.T) {
	result := ParseJSON(`{"api_key":"secret","command":"npx"}`)
	if result.Ok == nil {
		t.Fatalf("expected parse success, got error: %v", result.Err)
	}
	value, ok := result.Ok.Value.(map[string]any)
	if !ok {
		t.Fatalf("expected object value, got %T", result.Ok.Value)
	}
	if value["api_key"] != "[redacted]" {
		t.Fatalf("api_key = %v, want [redacted]", value["api_key"])
	}
	if value["command"] != "npx" {
		t.Fatalf("command = %v, want npx", value["command"])
	}
}

func TestParseTOMLHandlesMultilineArrays(t *testing.T) {
	text := `args = [
  "--app",
  "desktop",
]`
	result := ParseTOMLKeyValues(text)
	if result.Ok == nil {
		t.Fatalf("expected parse success, got error: %v", result.Err)
	}
	value, ok := result.Ok.Value.(map[string]any)
	if !ok {
		t.Fatalf("expected object value, got %T", result.Ok.Value)
	}
	want := []any{"--app", "desktop"}
	if !reflect.DeepEqual(value["args"], want) {
		t.Fatalf("args = %v, want %v", value["args"], want)
	}
}

func TestParseDotenvKeysClassifiesSecretLikeKeys(t *testing.T) {
	entries := ParseDotenvKeys("MODEL=gpt-5\nOPENAI_API_KEY=secret\n")
	if len(entries) != 2 {
		t.Fatalf("entries len = %d, want 2", len(entries))
	}
	if entries[0].CaptureStatus != "omitted" {
		t.Fatalf("entries[0].captureStatus = %q, want omitted", entries[0].CaptureStatus)
	}
	if entries[1].CaptureStatus != "redacted" {
		t.Fatalf("entries[1].captureStatus = %q, want redacted", entries[1].CaptureStatus)
	}
}
