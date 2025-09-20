package harmony

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync"

	"github.com/euforicio/harmony-go/tokenizer"
)

// EncodingName identifies the supported Harmony encodings.
type EncodingName string

// Supported encoding values.
const (
	HarmonyGptOss EncodingName = "HarmonyGptOss"
)

// Encoding provides rendering and parsing for the Harmony format using the
// O200k tokenizer with Harmony specials.
type Encoding struct {
	name string
	bpe  *tokenizer.Core // placeholder: expose via thin type alias if needed
	// formatting token mapping (string -> id)
	fmt map[string]uint32
	// cached formatting token ids (fast path)
	idStart     uint32
	idMessage   uint32
	idEnd       uint32
	idReturn    uint32
	idCall      uint32
	idConstrain uint32
	idChannel   uint32
	// stop token sets
	stopAll       map[uint32]struct{}
	stopAssistant map[uint32]struct{}
	builderPool   sync.Pool
	bufferPool    sync.Pool
}

// LoadEncoding returns an encoding by name. Only HarmonyGptOss is supported.
func LoadEncoding(name EncodingName) (*Encoding, error) {
	if name != HarmonyGptOss {
		return nil, fmt.Errorf("unsupported encoding: %s", name)
	}
	pairs, err := tokenizer.LoadO200k()
	if err != nil {
		return nil, err
	}
	seg := tokenizer.NewO200kSegmenter()
	bpe, err := tokenizer.NewCoreBPE(pairs, tokenizer.HarmonySpecials(), seg)
	if err != nil {
		return nil, err
	}
	fmtMap := map[string]uint32{
		"<|start|>":     tokenizer.TokStart,
		"<|message|>":   tokenizer.TokMessage,
		"<|end|>":       tokenizer.TokEnd,
		"<|return|>":    tokenizer.TokReturn,
		"<|call|>":      tokenizer.TokCall,
		"<|refusal|>":   0, // not used by mapping for HarmonyGptOss
		"<|constrain|>": tokenizer.TokConstrain,
		"<|channel|>":   tokenizer.TokChannel,
	}
	stopAll := map[uint32]struct{}{tokenizer.TokReturn: {}, tokenizer.TokCall: {}, tokenizer.TokEnd: {}}
	stopAssistant := map[uint32]struct{}{tokenizer.TokReturn: {}, tokenizer.TokCall: {}}
	enc := &Encoding{
		name:          string(name),
		bpe:           bpe,
		fmt:           fmtMap,
		stopAll:       stopAll,
		stopAssistant: stopAssistant,
		builderPool:   sync.Pool{New: func() any { return &strings.Builder{} }},
		bufferPool:    sync.Pool{New: func() any { return &bytes.Buffer{} }},
	}
	// cache ids
	enc.idStart = fmtMap["<|start|>"]
	enc.idMessage = fmtMap["<|message|>"]
	enc.idEnd = fmtMap["<|end|>"]
	enc.idReturn = fmtMap["<|return|>"]
	enc.idCall = fmtMap["<|call|>"]
	enc.idConstrain = fmtMap["<|constrain|>"]
	enc.idChannel = fmtMap["<|channel|>"]
	return enc, nil
}

// Name returns the encoding's canonical name.
func (e *Encoding) Name() string { return e.name }

// StopTokens returns the set of tokens that terminate any message.
func (e *Encoding) StopTokens() ([]uint32, error) {
	out := make([]uint32, 0, len(e.stopAll))
	for t := range e.stopAll {
		out = append(out, t)
	}
	return out, nil
}

// StopTokensForAssistantActions returns the stop tokens used for assistant
// actions (call/return) when streaming.
func (e *Encoding) StopTokensForAssistantActions() ([]uint32, error) {
	out := make([]uint32, 0, len(e.stopAssistant))
	for t := range e.stopAssistant {
		out = append(out, t)
	}
	return out, nil
}

// DecodeUTF8 decodes tokens into a UTF-8 string.
func (e *Encoding) DecodeUTF8(tokens []uint32) (string, error) {
	return e.bpe.DecodeUTF8(tokens)
}

// DecodeBytes decodes tokens into raw bytes.
func (e *Encoding) DecodeBytes(tokens []uint32) ([]byte, error) {
	return e.bpe.DecodeBytes(tokens)
}

// Render/Parse API stubs — implemented in subsequent steps.

type renderOptions struct {
	conversationHasFunctionTools bool
}

// Render encodes a single message into Harmony tokens.
func (e *Encoding) Render(msg Message) ([]uint32, error) {
	// default options
	return e.renderMessage(msg, renderOptions{})
}

func (e *Encoding) renderMessage(msg Message, opts renderOptions) ([]uint32, error) {
	var out []uint32
	if renderPresizeEnabled() {
		// Pre-size: rough estimate to cut growth churn for long messages
		capHint := estimateMessageSize(msg)/3 + 16
		if capHint > 1<<20 {
			capHint = 1 << 20
		}
		out = make([]uint32, 0, capHint)
	}
	// <|start|>
	if err := e.renderFormattingToken("<|start|>", &out); err != nil {
		return nil, err
	}

	if msg.Author.Role == RoleTool && msg.Author.Name == "" {
		return nil, fmt.Errorf("tool messages must have a name")
	}

	needsRecipient := msg.Recipient != "" && msg.Recipient != "all"
	switch msg.Author.Role {
	case RoleTool:
		if needsRecipient {
			e.renderText(msg.Author.Name, &out)
			e.renderText(" to=", &out)
			e.renderText(msg.Recipient, &out)
		} else {
			e.renderText(msg.Author.Name, &out)
		}
	default:
		if msg.Author.Name == "" && !needsRecipient {
			e.renderText(string(msg.Author.Role), &out)
		} else {
			e.renderText(string(msg.Author.Role), &out)
			if msg.Author.Name != "" {
				e.renderText(":", &out)
				e.renderText(msg.Author.Name, &out)
			}
			if needsRecipient {
				e.renderText(" to=", &out)
				e.renderText(msg.Recipient, &out)
			}
		}
	}

	// channel
	if msg.Channel != "" {
		if err := e.renderFormattingToken("<|channel|>", &out); err != nil {
			return nil, err
		}
		e.renderText(msg.Channel, &out)
	}

	// content-type
	if msg.ContentType != "" {
		e.renderContentType(msg.ContentType, &out)
	}

	// <|message|>
	if err := e.renderFormattingToken("<|message|>", &out); err != nil {
		return nil, err
	}

	// content
	for _, c := range msg.Content {
		switch c.Type {
		case ContentText:
			e.renderText(c.Text, &out)
		case ContentSystem:
			if c.System == nil {
				return nil, errors.New("nil SystemContent")
			}
			e.renderSystemContent(*c.System, opts, &out)
		case ContentDeveloper:
			if c.Developer == nil {
				return nil, errors.New("nil DeveloperContent")
			}
			e.renderDeveloperContent(*c.Developer, &out)
		default:
			return nil, fmt.Errorf("unknown content type: %v", c.Type)
		}
	}

	// end-of-message marker: assistant tool call uses <|call|>
	if msg.Author.Role == RoleAssistant && msg.Recipient != "" && msg.Recipient != "all" {
		if err := e.renderFormattingToken("<|call|>", &out); err != nil {
			return nil, err
		}
	} else {
		if err := e.renderFormattingToken("<|end|>", &out); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// RenderConversation encodes an entire conversation into Harmony tokens.
// When AutoDropAnalysis=true we omit analysis channel messages before the
// first final assistant message.
func (e *Encoding) RenderConversation(conv Conversation, cfg *RenderConversationConfig) ([]uint32, error) {
	autoDrop := true
	if cfg != nil {
		autoDrop = cfg.AutoDropAnalysis
	}

	// determine last assistant is final and first index of final
	lastAssistantFinal := false
	firstFinal := -1
	hasFunctionTools := false
	for i := range conv.Messages {
		m := conv.Messages[i]
		if m.Channel == "final" && firstFinal == -1 {
			firstFinal = i
		}
		if m.Author.Role == RoleAssistant {
			lastAssistantFinal = (m.Channel == "final")
		}
		if !hasFunctionTools {
			for _, c := range m.Content {
				if c.Type == ContentDeveloper && c.Developer != nil && c.Developer.Tools != nil {
					if ns, ok := c.Developer.Tools["functions"]; ok {
						if len(ns.Tools) > 0 {
							hasFunctionTools = true
							break
						}
					}
				}
			}
		}
	}
	shouldDrop := autoDrop && lastAssistantFinal

	renderIdx := make([]int, 0, len(conv.Messages))
	for i := range conv.Messages {
		m := conv.Messages[i]
		if shouldDrop && firstFinal >= 0 && i < firstFinal && m.Channel == "analysis" {
			continue
		}
		renderIdx = append(renderIdx, i)
	}
	if len(renderIdx) == 0 {
		return []uint32{}, nil
	}

	opts := renderOptions{conversationHasFunctionTools: hasFunctionTools}
	// Pre-size output token slice using a rough heuristic to reduce growth churn.
	estimateTokens := func(msg Message) int {
		chars := estimateMessageSize(msg)
		toks := chars/3 + 16
		if toks > 1<<20 {
			toks = 1 << 20
		}
		return toks
	}
	totalTokBudget := 0
	if renderPresizeEnabled() {
		for _, i := range renderIdx {
			totalTokBudget += estimateTokens(conv.Messages[i])
		}
	}
	if shouldParallelRender(conv.Messages, renderIdx) {
		results := make([][]uint32, len(renderIdx))
		var errOnce sync.Once
		var firstErr error
		maxWorkers := runtime.GOMAXPROCS(0)
		if maxWorkers < 1 {
			maxWorkers = 1
		}
		sem := make(chan struct{}, maxWorkers)
		var wg sync.WaitGroup
		for slot, idx := range renderIdx {
			wg.Add(1)
			sem <- struct{}{}
			go func(slot, msgIdx int) {
				defer wg.Done()
				defer func() { <-sem }()
				toks, err := e.renderMessage(conv.Messages[msgIdx], opts)
				if err != nil {
					errOnce.Do(func() { firstErr = err })
					return
				}
				results[slot] = toks
			}(slot, idx)
		}
		wg.Wait()
		if firstErr != nil {
			return nil, firstErr
		}
		var out []uint32
		if renderPresizeEnabled() {
			out = make([]uint32, 0, totalTokBudget)
		}
		for _, toks := range results {
			out = append(out, toks...)
		}
		return out, nil
	}

	var out []uint32
	if renderPresizeEnabled() {
		out = make([]uint32, 0, totalTokBudget)
	}
	for _, idx := range renderIdx {
		if err := e.renderMessageInto(conv.Messages[idx], opts, &out); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// RenderConversationForCompletion encodes a conversation and appends a
// <|start|>next-role header to prompt the model for the next message.
func (e *Encoding) RenderConversationForCompletion(conv Conversation, next Role, cfg *RenderConversationConfig) ([]uint32, error) {
	out, err := e.RenderConversation(conv, cfg)
	if err != nil {
		return nil, err
	}
	out = append(out, e.idStart)
	e.renderText(string(next), &out)
	return out, nil
}

// RenderConversationForTraining encodes a conversation replacing the trailing
// <|end|> with <|return|> when the last message is assistant:final.
func (e *Encoding) RenderConversationForTraining(conv Conversation, cfg *RenderConversationConfig) ([]uint32, error) {
	if len(conv.Messages) == 0 {
		return []uint32{}, nil
	}
	out, err := e.RenderConversation(conv, cfg)
	if err != nil {
		return nil, err
	}
	last := conv.Messages[len(conv.Messages)-1]
	if last.Author.Role == RoleAssistant && last.Channel == "final" {
		// replace trailing <|end|> with <|return|>
		if len(out) == 0 {
			return out, nil
		}
		out[len(out)-1] = e.idReturn
	}
	return out, nil
}

// ParseMessagesFromCompletionTokens parses completion tokens back into
// messages. If role is provided, it serves as a role hint for the first header.
func (e *Encoding) ParseMessagesFromCompletionTokens(tokens []uint32, role *Role) ([]Message, error) {
	p, err := NewStreamParser(e, role)
	if err != nil {
		return nil, err
	}
	for _, t := range tokens {
		if err := p.Process(t); err != nil {
			return nil, err
		}
	}
	if err := p.ProcessEOS(); err != nil {
		return nil, err
	}
	// Return messages slice directly to avoid a copy; parser is no longer used.
	return p.messages, nil
}

// internal helpers (to be used by render/parse)
func (e *Encoding) renderFormattingToken(name string, out *[]uint32) error {
	switch name {
	case "<|start|>":
		*out = append(*out, e.idStart)
		return nil
	case "<|message|>":
		*out = append(*out, e.idMessage)
		return nil
	case "<|end|>":
		*out = append(*out, e.idEnd)
		return nil
	case "<|return|>":
		*out = append(*out, e.idReturn)
		return nil
	case "<|call|>":
		*out = append(*out, e.idCall)
		return nil
	case "<|constrain|>":
		*out = append(*out, e.idConstrain)
		return nil
	case "<|channel|>":
		*out = append(*out, e.idChannel)
		return nil
	default:
		// slow path for future tokens
		id, ok := e.fmt[name]
		if !ok || id == 0 {
			return fmt.Errorf("unmapped formatting token %s", name)
		}
		*out = append(*out, id)
		return nil
	}
}

func (e *Encoding) renderText(text string, out *[]uint32) {
	_ = e.bpe.EncodeIntoOrdinary(text, out)
}

// renderMessageInto appends the rendered message tokens into out (no temp slice).
func (e *Encoding) renderMessageInto(msg Message, opts renderOptions, out *[]uint32) error {
	// <|start|>
	*out = append(*out, e.idStart)

	if msg.Author.Role == RoleTool && msg.Author.Name == "" {
		return fmt.Errorf("tool messages must have a name")
	}

	needsRecipient := msg.Recipient != "" && msg.Recipient != "all"
	switch msg.Author.Role {
	case RoleTool:
		if needsRecipient {
			header := msg.Author.Name + " to=" + msg.Recipient
			e.renderText(header, out)
		} else {
			e.renderText(msg.Author.Name, out)
		}
	default:
		if msg.Author.Name == "" && !needsRecipient {
			e.renderText(string(msg.Author.Role), out)
		} else {
			header := string(msg.Author.Role)
			if msg.Author.Name != "" {
				header = header + ":" + msg.Author.Name
			}
			if needsRecipient {
				header = header + " to=" + msg.Recipient
			}
			e.renderText(header, out)
		}
	}

	// channel
	if msg.Channel != "" {
		*out = append(*out, e.idChannel)
		e.renderText(msg.Channel, out)
	}

	// content-type
	if msg.ContentType != "" {
		e.renderContentType(msg.ContentType, out)
	}

	// <|message|>
	*out = append(*out, e.idMessage)

	// content
	for _, c := range msg.Content {
		switch c.Type {
		case ContentText:
			e.renderText(c.Text, out)
		case ContentSystem:
			if c.System == nil {
				return errors.New("nil SystemContent")
			}
			e.renderSystemContent(*c.System, opts, out)
		case ContentDeveloper:
			if c.Developer == nil {
				return errors.New("nil DeveloperContent")
			}
			e.renderDeveloperContent(*c.Developer, out)
		default:
			return fmt.Errorf("unknown content type: %v", c.Type)
		}
	}

	// end-of-message marker: assistant tool call uses <|call|>
	if msg.Author.Role == RoleAssistant && msg.Recipient != "" && msg.Recipient != "all" {
		*out = append(*out, e.idCall)
	} else {
		*out = append(*out, e.idEnd)
	}
	return nil
}

// EncodeWithSpecialTokens exposes tokenizer encoding with specials for tools.
// This is a convenience helper for benchmarks and CLIs.
func (e *Encoding) EncodeWithSpecialTokens(text string) []uint32 {
	return e.bpe.EncodeWithSpecialTokens(text)
}

// EncodeWithSpecialTokensInto appends tokens for text allowing Harmony specials.
// Zero-copy into out; returns the length of the last piece emitted.
func (e *Encoding) EncodeWithSpecialTokensInto(text string, out *[]uint32) int {
	return e.bpe.EncodeWithSpecialTokensInto(text, out)
}

// Special handling for content_type if it starts with <|constrain|>
func (e *Encoding) renderContentType(ct string, out *[]uint32) {
	if strings.HasPrefix(ct, "<|constrain|>") {
		// emit space, constrain special, then rest (if any)
		e.renderText(" ", out)
		*out = append(*out, e.idConstrain)
		rest := strings.TrimPrefix(ct, "<|constrain|>")
		if rest != "" {
			e.renderText(rest, out)
		}
		return
	}
	e.renderText(" "+ct, out)
}

const parallelRenderMinBytes = 8 * 1024
const parallelRenderMinMessages = 2

var (
	parallelFlag struct {
		once    sync.Once
		enabled bool
	}
	presizeFlag struct {
		once    sync.Once
		enabled bool
	}
)

func parallelRenderEnabled() bool {
	parallelFlag.once.Do(func() {
		v := strings.ToLower(os.Getenv("HARMONY_RENDER_PARALLEL"))
		if v == "0" || v == "false" {
			parallelFlag.enabled = false
		} else {
			parallelFlag.enabled = true
		}
	})
	return parallelFlag.enabled
}

func renderPresizeEnabled() bool {
	presizeFlag.once.Do(func() {
		v := strings.ToLower(os.Getenv("HARMONY_RENDER_PRESIZE"))
		if v == "0" || v == "false" {
			presizeFlag.enabled = false
		} else {
			presizeFlag.enabled = true
		}
	})
	return presizeFlag.enabled
}

func shouldParallelRender(msgs []Message, indices []int) bool {
	if !parallelRenderEnabled() {
		return false
	}
	if len(indices) < parallelRenderMinMessages {
		return false
	}
	total := 0
	for _, idx := range indices {
		total += estimateMessageSize(msgs[idx])
		if total >= parallelRenderMinBytes {
			return true
		}
	}
	return false
}

func estimateMessageSize(msg Message) int {
	total := len(msg.Author.Name) + len(msg.Channel) + len(msg.ContentType)
	if msg.Recipient != "" && msg.Recipient != "all" {
		total += len(msg.Recipient)
	}
	for _, c := range msg.Content {
		switch c.Type {
		case ContentText:
			total += len(c.Text)
		case ContentSystem:
			if c.System != nil {
				total += estimateSystemContentSize(c.System)
			}
		case ContentDeveloper:
			if c.Developer != nil {
				total += estimateDeveloperContentSize(c.Developer)
			}
		}
	}
	return total
}

func estimateSystemContentSize(sys *SystemContent) int {
	total := 0
	if sys.ModelIdentity != nil {
		total += len(*sys.ModelIdentity)
	}
	if sys.ReasoningEffort != nil {
		total += len(string(*sys.ReasoningEffort))
	}
	if sys.ConversationStartDate != nil {
		total += len(*sys.ConversationStartDate)
	}
	if sys.KnowledgeCutoff != nil {
		total += len(*sys.KnowledgeCutoff)
	}
	if sys.ChannelConfig != nil {
		total += estimateChannelConfigSize(sys.ChannelConfig)
	}
	total += estimateToolsMapSize(sys.Tools)
	return total
}

func estimateDeveloperContentSize(dev *DeveloperContent) int {
	total := 0
	if dev.Instructions != nil {
		total += len(*dev.Instructions)
	}
	total += estimateToolsMapSize(dev.Tools)
	return total
}

func estimateChannelConfigSize(cfg *ChannelConfig) int {
	total := 0
	for _, ch := range cfg.ValidChannels {
		total += len(ch)
	}
	// include boolean flag cost
	if cfg.ChannelRequired {
		total += 1
	}
	return total
}

func estimateToolsMapSize(tools map[string]ToolNamespaceConfig) int {
	total := 0
	for _, ns := range tools {
		total += len(ns.Name)
		if ns.Description != nil {
			total += len(*ns.Description)
		}
		// Iterate by index to avoid copying ToolDescription values.
		for i := range ns.Tools {
			td := &ns.Tools[i]
			total += len(td.Name)
			total += len(td.Description)
			total += len(td.Parameters)
		}
	}
	return total
}

func (e *Encoding) acquireBuilder() *strings.Builder {
	if v := e.builderPool.Get(); v != nil {
		b := v.(*strings.Builder)
		b.Reset()
		return b
	}
	return &strings.Builder{}
}

func (e *Encoding) releaseBuilder(b *strings.Builder) {
	b.Reset()
	e.builderPool.Put(b)
}

func (e *Encoding) acquireBuffer() *bytes.Buffer {
	if v := e.bufferPool.Get(); v != nil {
		buf := v.(*bytes.Buffer)
		buf.Reset()
		return buf
	}
	return &bytes.Buffer{}
}

func (e *Encoding) releaseBuffer(buf *bytes.Buffer) {
	buf.Reset()
	e.bufferPool.Put(buf)
}

func (e *Encoding) bufferStringAndRelease(buf *bytes.Buffer) string {
	res := buf.String()
	result := string(append([]byte(nil), res...))
	e.releaseBuffer(buf)
	return result
}
