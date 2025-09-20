package benchmarks

import (
	"strings"
	"testing"

	harmony "github.com/euforicio/harmony-go"
)

func BenchmarkRenderToolCall(b *testing.B) {
	b.ReportAllocs()
	enc := mustLoadEncoding(b)
	convo := toolCallConversation()
	cfg := &harmony.RenderConversationConfig{AutoDropAnalysis: true}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := enc.RenderConversationForCompletion(convo, harmony.RoleAssistant, cfg); err != nil {
			b.Fatalf("render: %v", err)
		}
	}
}

func BenchmarkRenderLargeAutoDrop(b *testing.B) {
	b.ReportAllocs()
	enc := mustLoadEncoding(b)
	convo := LargeConversation()
	cfg := &harmony.RenderConversationConfig{AutoDropAnalysis: true}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := enc.RenderConversationForCompletion(convo, harmony.RoleAssistant, cfg); err != nil {
			b.Fatalf("render: %v", err)
		}
	}
}

func BenchmarkRenderLargeKeepAnalysis(b *testing.B) {
	b.ReportAllocs()
	enc := mustLoadEncoding(b)
	convo := LargeConversation()
	cfg := &harmony.RenderConversationConfig{AutoDropAnalysis: false}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := enc.RenderConversationForCompletion(convo, harmony.RoleAssistant, cfg); err != nil {
			b.Fatalf("render: %v", err)
		}
	}
}

func BenchmarkParseToolCall(b *testing.B) {
	b.ReportAllocs()
	enc := mustLoadEncoding(b)
	tokens := encodeToolCallTokens(b, enc)
	author := harmony.RoleAssistant
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := enc.ParseMessagesFromCompletionTokens(tokens, &author); err != nil {
			b.Fatalf("parse: %v", err)
		}
	}
}

func BenchmarkStreamParseToolCall(b *testing.B) {
	b.ReportAllocs()
	enc := mustLoadEncoding(b)
	tokens := encodeToolCallTokens(b, enc)
	author := harmony.RoleAssistant
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		parser, err := harmony.NewStreamParser(enc, &author)
		if err != nil {
			b.Fatalf("stream parser: %v", err)
		}
		for _, tok := range tokens {
			if err := parser.Process(tok); err != nil {
				b.Fatalf("stream parse: %v", err)
			}
		}
		if err := parser.ProcessEOS(); err != nil {
			b.Fatalf("stream parse eos: %v", err)
		}
	}
}

func BenchmarkParseLargeCompletion(b *testing.B) {
	b.ReportAllocs()
	enc := mustLoadEncoding(b)
	tokens := largeCompletionTokens(b, enc)
	author := harmony.RoleAssistant
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := enc.ParseMessagesFromCompletionTokens(tokens, &author); err != nil {
			b.Fatalf("parse: %v", err)
		}
	}
}

func BenchmarkStreamParseLargeCompletion(b *testing.B) {
	b.ReportAllocs()
	enc := mustLoadEncoding(b)
	tokens := largeCompletionTokens(b, enc)
	author := harmony.RoleAssistant
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		parser, err := harmony.NewStreamParser(enc, &author)
		if err != nil {
			b.Fatalf("stream parser: %v", err)
		}
		for _, tok := range tokens {
			if err := parser.Process(tok); err != nil {
				b.Fatalf("stream parse: %v", err)
			}
		}
		if err := parser.ProcessEOS(); err != nil {
			b.Fatalf("stream parse eos: %v", err)
		}
	}
}

func mustLoadEncoding(tb testing.TB) *harmony.Encoding {
	tb.Helper()
	enc, err := harmony.LoadEncoding(harmony.HarmonyGptOss)
	if err != nil {
		tb.Fatalf("load encoding: %v", err)
	}
	return enc
}

func toolCallConversation() harmony.Conversation {
	return harmony.Conversation{Messages: []harmony.Message{
		{Author: harmony.Author{Role: harmony.RoleUser}, Content: []harmony.Content{{Type: harmony.ContentText, Text: "What is the weather in SF?"}}},
		{Author: harmony.Author{Role: harmony.RoleAssistant}, Channel: "analysis", Content: []harmony.Content{{Type: harmony.ContentText, Text: `User asks: "What is the weather in SF?" We need to use lookup_weather tool.`}}},
		{Author: harmony.Author{Role: harmony.RoleAssistant}, Channel: "commentary", Recipient: "functions.lookup_weather", ContentType: "<|constrain|>json", Content: []harmony.Content{{Type: harmony.ContentText, Text: `{"location": "San Francisco"}`}}},
		{Author: harmony.Author{Role: harmony.RoleTool, Name: "functions.lookup_weather"}, Content: []harmony.Content{{Type: harmony.ContentText, Text: `{"temperature": 20, "description": "sunny"}`}}},
	}}
}

func encodeToolCallTokens(tb testing.TB, enc *harmony.Encoding) []uint32 {
	tb.Helper()
	const toolCallText = "<|start|>assistant<|channel|>commentary to=functions.get_weather<|constrain|>json<|message|>{\"latitude\":48.8566,\"longitude\":2.3522}<|call|>"
	tokens := enc.EncodeWithSpecialTokens(toolCallText)
	if len(tokens) == 0 {
		tb.Fatalf("tool call tokens unexpectedly empty")
	}
	return tokens
}

func largeCompletionTokens(tb testing.TB, enc *harmony.Encoding) []uint32 {
	tb.Helper()
	analysis := harmony.Message{
		Author:  harmony.Author{Role: harmony.RoleAssistant},
		Channel: "analysis",
		Content: []harmony.Content{{Type: harmony.ContentText, Text: strings.Repeat("Reasoning chunk consolidating evidence. ", 60)}},
	}
	final := harmony.Message{
		Author:  harmony.Author{Role: harmony.RoleAssistant},
		Channel: "final",
		Content: []harmony.Content{{Type: harmony.ContentText, Text: "Final answer summarizing the conversation."}},
	}
	conv := harmony.Conversation{Messages: []harmony.Message{analysis, final}}
	cfg := &harmony.RenderConversationConfig{AutoDropAnalysis: false}
	tokens, err := enc.RenderConversation(conv, cfg)
	if err != nil {
		tb.Fatalf("render large completion: %v", err)
	}
	return tokens
}
