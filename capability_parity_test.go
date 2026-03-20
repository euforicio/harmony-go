package harmony

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/euforicio/harmony-go/tokenizer"
)

func readUpstreamFixture(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join("testdata", "upstream-head", name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", path, err)
	}
	return strings.TrimRight(string(data), "\n")
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	return string(data)
}

func TestRenderNoToolsFixture(t *testing.T) {
	enc := mustEncoding(t)

	conv := Conversation{Messages: []Message{
		{
			Author: Author{Role: RoleSystem},
			Content: []Content{{
				Type: ContentSystem,
				System: &SystemContent{
					ConversationStartDate: strPtr("2025-06-28"),
				},
			}},
		},
	}}

	tokens, err := enc.RenderConversationForCompletion(conv, RoleAssistant, nil)
	if err != nil {
		t.Fatalf("RenderConversationForCompletion: %v", err)
	}
	got, err := enc.DecodeUTF8(tokens)
	if err != nil {
		t.Fatalf("DecodeUTF8: %v", err)
	}
	want := readUpstreamFixture(t, "test_no_tools.txt")
	if got != want {
		t.Fatalf("render mismatch\n got:\n%s\nwant:\n%s", got, want)
	}
}

func TestRenderBrowserToolOnlyFixture(t *testing.T) {
	enc := mustEncoding(t)

	tools := map[string]ToolNamespaceConfig{
		"browser": BrowserToolNamespace(),
	}
	conv := Conversation{Messages: []Message{
		{
			Author: Author{Role: RoleSystem},
			Content: []Content{{
				Type: ContentSystem,
				System: &SystemContent{
					ConversationStartDate: strPtr("2025-06-28"),
					Tools:                 tools,
				},
			}},
		},
	}}

	tokens, err := enc.RenderConversationForCompletion(conv, RoleAssistant, nil)
	if err != nil {
		t.Fatalf("RenderConversationForCompletion: %v", err)
	}
	got, err := enc.DecodeUTF8(tokens)
	if err != nil {
		t.Fatalf("DecodeUTF8: %v", err)
	}
	want := readUpstreamFixture(t, "test_browser_tool_only.txt")
	if got != want {
		t.Fatalf("render mismatch\n got:\n%s\nwant:\n%s", got, want)
	}
}

func TestRenderBrowserAndPythonToolFixture(t *testing.T) {
	enc := mustEncoding(t)

	tools := map[string]ToolNamespaceConfig{
		"browser": BrowserToolNamespace(),
		"python":  PythonToolNamespace(),
	}
	conv := Conversation{Messages: []Message{
		{
			Author: Author{Role: RoleSystem},
			Content: []Content{{
				Type: ContentSystem,
				System: &SystemContent{
					ConversationStartDate: strPtr("2025-06-28"),
					Tools:                 tools,
				},
			}},
		},
	}}

	tokens, err := enc.RenderConversationForCompletion(conv, RoleAssistant, nil)
	if err != nil {
		t.Fatalf("RenderConversationForCompletion: %v", err)
	}
	got, err := enc.DecodeUTF8(tokens)
	if err != nil {
		t.Fatalf("DecodeUTF8: %v", err)
	}
	want := readUpstreamFixture(t, "test_browser_and_python_tool.txt")
	if got != want {
		t.Fatalf("render mismatch\n got:\n%s\nwant:\n%s", got, want)
	}
}

func TestKeepAnalysisBetweenFinalMessages(t *testing.T) {
	enc := mustEncoding(t)

	conv := Conversation{Messages: []Message{
		{Author: Author{Role: RoleUser}, Content: []Content{{Type: ContentText, Text: "What is 2 + 2?"}}},
		{Author: Author{Role: RoleAssistant}, Channel: "analysis", Content: []Content{{Type: ContentText, Text: "thinking 2+2"}}},
		{Author: Author{Role: RoleAssistant}, Channel: "final", Content: []Content{{Type: ContentText, Text: "4"}}},
		{Author: Author{Role: RoleUser}, Content: []Content{{Type: ContentText, Text: "What is 3 + 5?"}}},
		{Author: Author{Role: RoleAssistant}, Channel: "analysis", Content: []Content{{Type: ContentText, Text: "thinking 3+5"}}},
		{Author: Author{Role: RoleAssistant}, Channel: "final", Content: []Content{{Type: ContentText, Text: "8"}}},
	}}

	tokens, err := enc.RenderConversation(conv, nil)
	if err != nil {
		t.Fatalf("RenderConversation: %v", err)
	}
	got, err := enc.DecodeUTF8(tokens)
	if err != nil {
		t.Fatalf("DecodeUTF8: %v", err)
	}
	want := readUpstreamFixture(t, "test_keep_analysis_between_finals.txt")
	if got != want {
		t.Fatalf("render mismatch\n got:\n%s\nwant:\n%s", got, want)
	}
}

func TestEncodeAllowedDisallowedSpecial(t *testing.T) {
	enc := mustEncoding(t)

	hello, err := enc.Encode("hello world", nil)
	if err != nil {
		t.Fatalf("Encode ordinary: %v", err)
	}
	if want := []uint32{24912, 2375}; !slices.Equal(hello, want) {
		t.Fatalf("ordinary encode mismatch\n got: %v\nwant: %v", hello, want)
	}

	allowed := map[string]struct{}{"<|start|>": {}}
	got, err := enc.Encode("<|start|>", &EncodeOptions{AllowedSpecial: allowed})
	if err != nil {
		t.Fatalf("Encode allowed special: %v", err)
	}
	if want := []uint32{tokenizer.TokStart}; !slices.Equal(got, want) {
		t.Fatalf("allowed special mismatch\n got: %v\nwant: %v", got, want)
	}

	if _, err := enc.Encode("<|start|>", nil); err == nil {
		t.Fatalf("expected disallowed special error")
	}

	got, err = enc.Encode("<|start|>", &EncodeOptions{DisallowedSpecial: map[string]struct{}{}})
	if err != nil {
		t.Fatalf("Encode natural special text: %v", err)
	}
	wantNatural := []uint32{27, 91, 5236, 91, 29}
	if !slices.Equal(got, wantNatural) {
		t.Fatalf("natural special mismatch\n got: %v\nwant: %v", got, wantNatural)
	}
}

func TestDecodeReplaceAndIsSpecialToken(t *testing.T) {
	enc := mustEncoding(t)

	if !enc.IsSpecialToken(tokenizer.TokStart) {
		t.Fatalf("TokStart should be special")
	}
	if enc.IsSpecialToken(24912) {
		t.Fatalf("24912 should not be special")
	}

	if got := enc.Decode([]uint32{24912, 2375}); got != "hello world" {
		t.Fatalf("Decode ordinary = %q", got)
	}

	if _, err := enc.DecodeUTF8([]uint32{132990, 9552}); err == nil {
		t.Fatalf("expected invalid utf-8 error")
	}
	if got := enc.Decode([]uint32{132990, 9552}); !strings.Contains(got, "Chicken") {
		t.Fatalf("Decode should replace invalid utf-8, got %q", got)
	}
}

func TestParseMessagesWithOptionsNonStrictRecovers(t *testing.T) {
	enc := mustEncoding(t)

	tokens, err := enc.Encode("I must refuse<|end|><|start|>assistant<|channel|>analysis<|message|>We must refuse<|end|>", &EncodeOptions{
		AllowedSpecial: AllSpecialTokens(),
	})
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	role := RoleAssistant
	if _, err := enc.ParseMessagesFromCompletionTokensWithOptions(tokens, &role, ParseOptions{Strict: true}); err == nil {
		t.Fatalf("expected strict parse failure")
	}

	msgs, err := enc.ParseMessagesFromCompletionTokensWithOptions(tokens, &role, ParseOptions{Strict: false})
	if err != nil {
		t.Fatalf("ParseMessagesFromCompletionTokensWithOptions non-strict: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	if msgs[0].Content[0].Text != "I must refuse" {
		t.Fatalf("unexpected recovered content: %+v", msgs[0])
	}
}

func TestStreamParserWithOptionsInvalidUTF8Recovery(t *testing.T) {
	enc := mustEncoding(t)

	prefix, err := enc.Encode("<|start|>assistant<|message|>", &EncodeOptions{AllowedSpecial: AllSpecialTokens()})
	if err != nil {
		t.Fatalf("Encode prefix: %v", err)
	}
	suffix, err := enc.Encode("worked<|end|>", &EncodeOptions{AllowedSpecial: AllSpecialTokens()})
	if err != nil {
		t.Fatalf("Encode suffix: %v", err)
	}
	tokens := append(append(prefix, 9552, 9552), suffix...)

	parser, err := NewStreamParserWithOptions(enc, nil, ParseOptions{Strict: true})
	if err != nil {
		t.Fatalf("NewStreamParserWithOptions: %v", err)
	}
	for _, tok := range tokens {
		if err := parser.Process(tok); err != nil {
			t.Fatalf("Process: %v", err)
		}
	}
	msgs := parser.Messages()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].Content[0].Text != " \uFFFD \uFFFDworked" {
		t.Fatalf("unexpected recovered content: %q", msgs[0].Content[0].Text)
	}
}

func TestStreamParserNonStrictRoleHintDoesNotDuplicateTokens(t *testing.T) {
	enc := mustEncoding(t)

	tokens, err := enc.Encode("second<|end|>", &EncodeOptions{
		AllowedSpecial: AllSpecialTokens(),
	})
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	role := RoleAssistant
	parser, err := NewStreamParserWithOptions(enc, &role, ParseOptions{Strict: false})
	if err != nil {
		t.Fatalf("NewStreamParserWithOptions: %v", err)
	}
	parser.state = stExpectStart
	for _, tok := range tokens {
		if err := parser.Process(tok); err != nil {
			t.Fatalf("Process: %v", err)
		}
	}
	if err := parser.ProcessEOS(); err != nil {
		t.Fatalf("ProcessEOS: %v", err)
	}

	if got := parser.Tokens(); !slices.Equal(got, tokens) {
		t.Fatalf("Tokens() duplicated or dropped tokens: got %v want %v", got, tokens)
	}
	msgs := parser.Messages()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].Content[0].Text != "second" {
		t.Fatalf("unexpected recovered messages: %+v", msgs)
	}
}

func TestStreamParserStrictRejectsStopTokenInHeader(t *testing.T) {
	enc := mustEncoding(t)

	tokens, err := enc.Encode("<|start|>assistant<|end|>", &EncodeOptions{
		AllowedSpecial: AllSpecialTokens(),
	})
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	parser, err := NewStreamParserWithOptions(enc, nil, ParseOptions{Strict: true})
	if err != nil {
		t.Fatalf("NewStreamParserWithOptions: %v", err)
	}
	for _, tok := range tokens {
		err = parser.Process(tok)
		if err != nil {
			break
		}
	}
	if err == nil || !strings.Contains(err.Error(), "unexpected stop token in message header") {
		t.Fatalf("expected strict header stop error, got %v", err)
	}
}

func TestStreamParserNonStrictRecoveryClearsRoleHint(t *testing.T) {
	enc := mustEncoding(t)

	tokens, err := enc.Encode("broken<|end|><|start|>functions.lookup_weather<|message|>{\"ok\":true}<|end|>", &EncodeOptions{
		AllowedSpecial: AllSpecialTokens(),
	})
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	role := RoleAssistant
	parser, err := NewStreamParserWithOptions(enc, &role, ParseOptions{Strict: false})
	if err != nil {
		t.Fatalf("NewStreamParserWithOptions: %v", err)
	}
	for _, tok := range tokens {
		if err := parser.Process(tok); err != nil {
			t.Fatalf("Process: %v", err)
		}
	}
	if err := parser.ProcessEOS(); err != nil {
		t.Fatalf("ProcessEOS: %v", err)
	}

	msgs := parser.Messages()
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	if msgs[0].Author.Role != RoleAssistant {
		t.Fatalf("unexpected recovered role: %+v", msgs[0].Author)
	}
	if msgs[1].Author.Role != RoleTool || msgs[1].Author.Name != "functions.lookup_weather" {
		t.Fatalf("expected tool message after recovery, got %+v", msgs[1].Author)
	}
}

func TestCLIParseRoleIsOptional(t *testing.T) {
	enc := mustEncoding(t)

	text := "<|start|>browser.search<|message|>{\"ok\":true}<|end|>"
	tokens, err := enc.Encode(text, &EncodeOptions{AllowedSpecial: AllSpecialTokens()})
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	cmd := exec.Command("go", "run", "./cmd/harmony-go", "parse")
	cmd.Dir = "."
	cmd.Stdin = strings.NewReader(mustJSON(t, tokens))
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go run parse: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "\"name\":\"browser.search\"") {
		t.Fatalf("expected tool name in parse output, got %s", out)
	}
}
