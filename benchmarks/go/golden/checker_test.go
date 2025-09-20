package golden

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/euforicio/harmony-go"
)

func TestRenderMatchesGolden(t *testing.T) {
	enc, err := harmony.LoadEncoding(harmony.HarmonyGptOss)
	if err != nil {
		t.Fatalf("load encoding: %v", err)
	}

	type renderFn func(*harmony.Encoding, harmony.Conversation, *harmony.RenderConversationConfig, *harmony.Role) ([]uint32, error)

	renderCompletion := func(enc *harmony.Encoding, convo harmony.Conversation, cfg *harmony.RenderConversationConfig, role *harmony.Role) ([]uint32, error) {
		if role == nil {
			return nil, errMissingRole("completion")
		}
		return enc.RenderConversationForCompletion(convo, *role, cfg)
	}

	renderTraining := func(enc *harmony.Encoding, convo harmony.Conversation, cfg *harmony.RenderConversationConfig, _ *harmony.Role) ([]uint32, error) {
		return enc.RenderConversationForTraining(convo, cfg)
	}

	renderConversation := func(enc *harmony.Encoding, convo harmony.Conversation, cfg *harmony.RenderConversationConfig, _ *harmony.Role) ([]uint32, error) {
		return enc.RenderConversation(convo, cfg)
	}

	modelID := "gpt-test"
	reasoning := harmony.ReasoningMedium
	startDate := "2025-09-01"
	cutoff := "2023-10-01"
	toolDesc := harmony.ToolDescription{
		Name:        "get_weather",
		Description: "Lookup the forecast",
		Parameters:  json.RawMessage(`{"type":"object","properties":{"location":{"type":"string"}}}`),
	}
	systemTools := map[string]harmony.ToolNamespaceConfig{
		"functions": {
			Name:        "functions",
			Description: strPtr("Call JSON functions"),
			Tools:       []harmony.ToolDescription{toolDesc},
		},
	}
	devInstructions := "Prefer metric units."
	devTools := map[string]harmony.ToolNamespaceConfig{
		"functions": {
			Name:  "functions",
			Tools: []harmony.ToolDescription{toolDesc},
		},
	}

	cases := []struct {
		name   string
		file   string
		convo  harmony.Conversation
		role   *harmony.Role
		config *harmony.RenderConversationConfig
		render renderFn
	}{
		{
			name: "tool_call",
			file: "tool_call.tokens.json",
			convo: harmony.Conversation{Messages: []harmony.Message{
				{Author: harmony.Author{Role: harmony.RoleUser}, Content: []harmony.Content{{Type: harmony.ContentText, Text: "What is the weather in SF?"}}},
				{Author: harmony.Author{Role: harmony.RoleAssistant}, Channel: "analysis", Content: []harmony.Content{{Type: harmony.ContentText, Text: `User asks: "What is the weather in SF?" We need to use lookup_weather tool.`}}},
				{Author: harmony.Author{Role: harmony.RoleAssistant}, Channel: "commentary", Recipient: "functions.lookup_weather", ContentType: "<|constrain|>json", Content: []harmony.Content{{Type: harmony.ContentText, Text: `{"location": "San Francisco"}`}}},
				{Author: harmony.Author{Role: harmony.RoleTool, Name: "functions.lookup_weather"}, Content: []harmony.Content{{Type: harmony.ContentText, Text: `{"temperature": 20, "description": "sunny"}`}}},
			}},
			role:   rolePtr(harmony.RoleAssistant),
			config: &harmony.RenderConversationConfig{AutoDropAnalysis: true},
			render: renderCompletion,
		},
		{
			name: "system_developer",
			file: "system_developer.tokens.json",
			convo: harmony.Conversation{Messages: []harmony.Message{
				{Author: harmony.Author{Role: harmony.RoleSystem}, Content: []harmony.Content{{Type: harmony.ContentSystem, System: &harmony.SystemContent{
					ModelIdentity:         &modelID,
					ReasoningEffort:       &reasoning,
					Tools:                 systemTools,
					ConversationStartDate: &startDate,
					KnowledgeCutoff:       &cutoff,
					ChannelConfig: &harmony.ChannelConfig{
						ValidChannels:   []string{"analysis", "commentary", "final"},
						ChannelRequired: true,
					},
				}}}},
				{Author: harmony.Author{Role: harmony.RoleDeveloper}, Content: []harmony.Content{{Type: harmony.ContentDeveloper, Developer: &harmony.DeveloperContent{
					Instructions: &devInstructions,
					Tools:        devTools,
				}}}},
				{Author: harmony.Author{Role: harmony.RoleUser}, Content: []harmony.Content{{Type: harmony.ContentText, Text: "Plan a day in San Francisco with food and sights."}}},
			}},
			role:   rolePtr(harmony.RoleAssistant),
			render: renderCompletion,
		},
		{
			name: "training_return",
			file: "training_return.tokens.json",
			convo: harmony.Conversation{Messages: []harmony.Message{
				{Author: harmony.Author{Role: harmony.RoleUser}, Content: []harmony.Content{{Type: harmony.ContentText, Text: "Ping"}}},
				{Author: harmony.Author{Role: harmony.RoleAssistant}, Channel: "final", Content: []harmony.Content{{Type: harmony.ContentText, Text: "Pong"}}},
			}},
			render: renderTraining,
		},
		{
			name: "constrain_json",
			file: "constrain_json.tokens.json",
			convo: harmony.Conversation{Messages: []harmony.Message{
				{Author: harmony.Author{Role: harmony.RoleAssistant}, Channel: "commentary", ContentType: "<|constrain|>json", Content: []harmony.Content{{Type: harmony.ContentText, Text: `{"foo": 1}`}}},
			}},
			render: renderConversation,
			config: &harmony.RenderConversationConfig{AutoDropAnalysis: false},
		},
		{
			name: "autodrop_true",
			file: "autodrop_true.tokens.json",
			convo: harmony.Conversation{Messages: []harmony.Message{
				{Author: harmony.Author{Role: harmony.RoleUser}, Content: []harmony.Content{{Type: harmony.ContentText, Text: "Explain the steps"}}},
				{Author: harmony.Author{Role: harmony.RoleAssistant}, Channel: "analysis", Content: []harmony.Content{{Type: harmony.ContentText, Text: "Reasoning in progress"}}},
				{Author: harmony.Author{Role: harmony.RoleAssistant}, Channel: "commentary", Recipient: "functions.call", Content: []harmony.Content{{Type: harmony.ContentText, Text: "tool call"}}},
				{Author: harmony.Author{Role: harmony.RoleTool, Name: "functions.call"}, Content: []harmony.Content{{Type: harmony.ContentText, Text: "{}"}}},
				{Author: harmony.Author{Role: harmony.RoleAssistant}, Channel: "final", Content: []harmony.Content{{Type: harmony.ContentText, Text: "Here you go"}}},
			}},
			render: renderConversation,
			config: &harmony.RenderConversationConfig{AutoDropAnalysis: true},
		},
		{
			name: "autodrop_false",
			file: "autodrop_false.tokens.json",
			convo: harmony.Conversation{Messages: []harmony.Message{
				{Author: harmony.Author{Role: harmony.RoleUser}, Content: []harmony.Content{{Type: harmony.ContentText, Text: "Explain the steps"}}},
				{Author: harmony.Author{Role: harmony.RoleAssistant}, Channel: "analysis", Content: []harmony.Content{{Type: harmony.ContentText, Text: "Reasoning in progress"}}},
				{Author: harmony.Author{Role: harmony.RoleAssistant}, Channel: "commentary", Recipient: "functions.call", Content: []harmony.Content{{Type: harmony.ContentText, Text: "tool call"}}},
				{Author: harmony.Author{Role: harmony.RoleTool, Name: "functions.call"}, Content: []harmony.Content{{Type: harmony.ContentText, Text: "{}"}}},
				{Author: harmony.Author{Role: harmony.RoleAssistant}, Channel: "final", Content: []harmony.Content{{Type: harmony.ContentText, Text: "Here you go"}}},
			}},
			render: renderConversation,
			config: &harmony.RenderConversationConfig{AutoDropAnalysis: false},
		},
	}

	update := os.Getenv("GOLDEN_UPDATE") == "1"

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			renderer := tc.render
			if renderer == nil {
				renderer = renderCompletion
			}
			tokens, err := renderer(enc, tc.convo, tc.config, tc.role)
			if err != nil {
				t.Fatalf("render: %v", err)
			}

			goldenPath := filepath.Join("testdata", tc.file)
			if update {
				writeGolden(t, goldenPath, tokens)
				return
			}

			expected := readGolden(t, goldenPath)
			if len(tokens) != len(expected) {
				t.Fatalf("token length mismatch: got %d, want %d", len(tokens), len(expected))
			}
			for i := range tokens {
				if tokens[i] != expected[i] {
					t.Fatalf("token mismatch at %d: got %d, want %d", i, tokens[i], expected[i])
				}
			}
		})
	}
}

func readGolden(t *testing.T, path string) []uint32 {
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s: %v", path, err)
	}
	var out []uint32
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal golden %s: %v", path, err)
	}
	return out
}

func writeGolden(t *testing.T, path string, tokens []uint32) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	encoded, err := json.MarshalIndent(tokens, "", "  ")
	if err != nil {
		t.Fatalf("marshal golden: %v", err)
	}
	if err := os.WriteFile(path, append(encoded, '\n'), 0o644); err != nil {
		t.Fatalf("write golden %s: %v", path, err)
	}
}

func strPtr(s string) *string {
	return &s
}

func rolePtr(r harmony.Role) *harmony.Role {
	return &r
}

func errMissingRole(mode string) error {
	return fmt.Errorf("role is required for %s rendering", mode)
}

// end
