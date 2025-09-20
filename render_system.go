package harmony

import "strings"

// renderSystemContent renders the system content block: identity, dates, reasoning,
// tools section headers and channel metadata directly into the token stream.
func (e *Encoding) renderSystemContent(sys SystemContent, opts renderOptions, out *[]uint32) {
	body := e.acquireBuilder()
	// Pre-size to reduce reallocations; heuristic using estimators
	// The estimators approximate source sizes; double for formatting overhead.
	if sz := estimateSystemContentSize(&sys); sz > 0 {
		// cap to a reasonable bound to avoid huge over-allocation
		if sz > 1<<18 { // 256 KiB
			sz = 1 << 18
		}
		body.Grow(sz*2 + 128)
	}
	addSection := func(write func(*strings.Builder)) {
		if body.Len() > 0 {
			body.WriteString("\n\n")
		}
		write(body)
	}

	mid := "You are ChatGPT, a large language model trained by OpenAI."
	if sys.ModelIdentity != nil && *sys.ModelIdentity != "" {
		mid = *sys.ModelIdentity
	}
	kc := "2024-06"
	if sys.KnowledgeCutoff != nil && *sys.KnowledgeCutoff != "" {
		kc = *sys.KnowledgeCutoff
	}
	addSection(func(sb *strings.Builder) {
		sb.WriteString(mid)
		sb.WriteByte('\n')
		sb.WriteString("Knowledge cutoff: ")
		sb.WriteString(kc)
		if sys.ConversationStartDate != nil && *sys.ConversationStartDate != "" {
			sb.WriteByte('\n')
			sb.WriteString("Current date: ")
			sb.WriteString(*sys.ConversationStartDate)
		}
	})

	eff := "medium"
	if sys.ReasoningEffort != nil {
		eff = strings.ToLower(string(*sys.ReasoningEffort))
	}
	addSection(func(sb *strings.Builder) {
		sb.WriteString("Reasoning: ")
		sb.WriteString(eff)
	})

	if len(sys.Tools) > 0 {
		addSection(func(sb *strings.Builder) {
			e.writeToolsSection(sb, sys.Tools)
		})
	}

	chanCfg := sys.ChannelConfig
	if chanCfg == nil {
		chanCfg = &ChannelConfig{ValidChannels: []string{"analysis", "commentary", "final"}, ChannelRequired: true}
	}
	if len(chanCfg.ValidChannels) > 0 {
		channels := strings.Join(chanCfg.ValidChannels, ", ")
		addSection(func(sb *strings.Builder) {
			sb.WriteString("# Valid channels: ")
			sb.WriteString(channels)
			sb.WriteString(".")
			if chanCfg.ChannelRequired {
				sb.WriteString(" Channel must be included for every message.")
			}
			if opts.conversationHasFunctionTools {
				sb.WriteString("\nCalls to these tools must go to the commentary channel: 'functions'.")
			}
		})
	}

	e.renderText(body.String(), out)
	e.releaseBuilder(body)
}
