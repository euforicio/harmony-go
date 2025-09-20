package harmony

import "testing"

func TestNormalizeHeader(t *testing.T) {
	in := "assistant to=functions.get_weather<|channel|>commentary<|constrain|>json"
	got := normalizeHeader(in)
	want := "assistant to=functions.get_weather <|channel|>commentary <|constrain|>json"
	if got != want {
		t.Fatalf("normalizeHeader: got %q want %q", got, want)
	}
}

func TestSplitLeadingToken(t *testing.T) {
	tok, rem := splitLeadingToken("assistant<|channel|>analysis")
	if tok != "assistant" || rem != "<|channel|>analysis" {
		t.Fatalf("splitLeadingToken unexpected: %q %q", tok, rem)
	}
}

func TestDetectRoleAndAuthor(t *testing.T) {
	// assistant alias
	r, name := detectRoleAndAuthor("assistant:math", "<|channel|>analysis")
	if r != RoleAssistant || name != "math" {
		t.Fatalf("assistant alias: got (%v,%q)", r, name)
	}
	// plain assistant
	r, name = detectRoleAndAuthor("assistant", "to=functions.foo")
	if r != RoleAssistant || name != "" {
		t.Fatalf("assistant: got (%v,%q)", r, name)
	}
	// implicit tool name
	r, name = detectRoleAndAuthor("functions.lookup_weather", "<|channel|>commentary")
	if r != RoleTool || name != "functions.lookup_weather" {
		t.Fatalf("tool implicit: got (%v,%q)", r, name)
	}
	// explicit tool prefix
	r, name = detectRoleAndAuthor("tool:browser.search", "")
	if r != RoleTool || name != "browser.search" {
		t.Fatalf("tool explicit: got (%v,%q)", r, name)
	}
}

func TestExtractors(t *testing.T) {
	s := "assistant to=functions.get_weather<|channel|>commentary <|constrain|>json"
	if ch := extractChannel(s); ch != "commentary" {
		t.Fatalf("extractChannel: %q", ch)
	}
	if rcpt := extractRecipient(s); rcpt != "functions.get_weather" {
		t.Fatalf("extractRecipient: %q", rcpt)
	}
}

func TestScrubContentType(t *testing.T) {
	role := "assistant"
	rem := "to=functions.get_weather<|channel|>commentary <|constrain|>json"
	if ct := scrubContentType(role, rem); ct != "<|constrain|>json" {
		t.Fatalf("scrubContentType: %q", ct)
	}
}
