package harmony

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/euforicio/harmony-go/tokenizer"
)

type streamState int

const (
	stExpectStart streamState = iota
	stHeader
	stContent
)

type parsedHeader struct {
	author      Author
	recipient   string
	channel     string
	contentType string
}

// StreamParser incrementally parses Harmony tokens into messages. It mirrors
// the behavior of the upstream StreamableParser and is useful for streaming.
type StreamParser struct {
	enc          *Encoding
	nextRole     *Role
	options      ParseOptions
	state        streamState
	tokens       []uint32
	messages     []Message
	headerToks   []uint32
	contentBytes []byte
	// last delta stored as bytes to avoid per-token string allocs
	lastDeltaBytes []byte
	// scratch buffer reused for per-token decoding to reduce allocations
	scratch        []byte
	undecodedBytes []byte
}

// NewStreamParser creates a streaming parser. If role is provided, it is used
// as a hint for the upcoming header and the parser starts in Header state.
func NewStreamParser(enc *Encoding, role *Role) (*StreamParser, error) {
	return NewStreamParserWithOptions(enc, role, ParseOptions{Strict: true})
}

// NewStreamParserWithOptions creates a streaming parser with explicit parse options.
func NewStreamParserWithOptions(enc *Encoding, role *Role, opts ParseOptions) (*StreamParser, error) {
	st := stExpectStart
	if role != nil {
		// Match upstream behaviour: if a next role is hinted, begin collecting header tokens
		// immediately until we see <|message|>.
		st = stHeader
	}
	return &StreamParser{
		enc:            enc,
		nextRole:       role,
		options:        opts,
		state:          st,
		tokens:         make([]uint32, 0, 16),
		messages:       make([]Message, 0, 2),
		headerToks:     make([]uint32, 0, 8),
		contentBytes:   make([]byte, 0, 64),
		lastDeltaBytes: make([]byte, 0, 32),
		scratch:        make([]byte, 0, 16),
		undecodedBytes: make([]byte, 0, 8),
	}, nil
}

// Process consumes a single token and updates the parser state.
func (p *StreamParser) Process(token uint32) error {
	p.tokens = append(p.tokens, token)
	switch p.state {
	case stExpectStart:
		if token == tokenizer.TokStart {
			p.headerToks = p.headerToks[:0]
			p.state = stHeader
			return nil
		}
		if !p.options.Strict && p.nextRole != nil {
			p.headerToks = p.headerToks[:0]
			p.contentBytes = p.contentBytes[:0]
			p.undecodedBytes = p.undecodedBytes[:0]
			p.messages = append(p.messages, Message{Author: Author{Role: *p.nextRole}})
			p.state = stContent
			if _, stop := p.enc.stopAll[token]; stop {
				if err := p.finalizeMessage(); err != nil {
					return err
				}
				p.state = stExpectStart
				return nil
			}
			return p.consumeContentToken(token)
		}
		return errors.New("unexpected token while expecting <|start|>")
	case stHeader:
		if _, stop := p.enc.stopAll[token]; stop {
			if p.options.Strict {
				return errors.New("unexpected stop token in message header")
			}
			p.messages = append(p.messages, Message{
				Author:  Author{Role: derefRole(p.nextRole, RoleAssistant)},
				Content: []Content{{Type: ContentText, Text: mustDecodeLossy(p.enc, p.headerToks)}},
			})
			p.headerToks = p.headerToks[:0]
			p.state = stExpectStart
			return nil
		}
		if token == tokenizer.TokStart {
			// Ignore stray start tokens when beginning in Header due to role hint
			if len(p.headerToks) > 0 {
				if p.options.Strict {
					return errors.New("unexpected tokens remaining in message header")
				}
				p.messages = append(p.messages, Message{
					Author:  Author{Role: derefRole(p.nextRole, RoleAssistant)},
					Content: []Content{{Type: ContentText, Text: mustDecodeLossy(p.enc, p.headerToks)}},
				})
				p.headerToks = p.headerToks[:0]
			}
			return nil
		}
		if token == tokenizer.TokMessage {
			// parse header tokens
			hdr, remaining, err := p.parseHeaderFromTokens(p.headerToks)
			if err != nil {
				return err
			}
			// set state
			p.nextRole = nil
			p.contentBytes = p.contentBytes[:0]
			p.undecodedBytes = p.undecodedBytes[:0]
			// store header in next message via zero-width marker: we carry as separate field? we'll stash in struct
			// Encapsulate header in a new message placeholder using content later
			p.messages = append(p.messages, Message{Author: hdr.author, Recipient: hdr.recipient, Channel: hdr.channel, ContentType: hdr.contentType})
			if remaining != "" {
				p.renderRecoveredText(remaining)
			}
			p.state = stContent
			return nil
		}
		p.headerToks = append(p.headerToks, token)
		return nil
	case stContent:
		// stop tokens finalize message
		if _, stop := p.enc.stopAll[token]; stop {
			if err := p.finalizeMessage(); err != nil {
				return err
			}
			p.state = stExpectStart
			return nil
		}
		// Append token to logical content
		return p.consumeContentToken(token)
	default:
		return errors.New("invalid parser state")
	}
}

func (p *StreamParser) finalizeMessage() error {
	if len(p.messages) == 0 {
		return nil
	}
	idx := len(p.messages) - 1
	text := string(p.contentBytes)
	if len(p.undecodedBytes) > 0 {
		text += string(bytes.Runes(p.undecodedBytes))
	}
	p.messages[idx].Content = []Content{{Type: ContentText, Text: text}}
	// reset buffers
	p.headerToks = p.headerToks[:0]
	p.contentBytes = p.contentBytes[:0]
	p.undecodedBytes = p.undecodedBytes[:0]
	p.lastDeltaBytes = p.lastDeltaBytes[:0]
	return nil
}

// ProcessEOS flushes any buffered content and finalizes the current message.
func (p *StreamParser) ProcessEOS() error {
	if p.state == stContent {
		return p.finalizeMessage()
	}
	if p.state == stHeader && len(p.headerToks) > 0 {
		if p.options.Strict {
			return errors.New("unexpected end of stream in message header")
		}
		p.messages = append(p.messages, Message{
			Author:  Author{Role: derefRole(p.nextRole, RoleAssistant)},
			Content: []Content{{Type: ContentText, Text: mustDecodeLossy(p.enc, p.headerToks)}},
		})
		p.headerToks = p.headerToks[:0]
		p.state = stExpectStart
	}
	return nil
}

// Messages returns all fully parsed messages so far.
func (p *StreamParser) Messages() []Message { return append([]Message(nil), p.messages...) }

// Tokens returns all tokens that have been fed to the parser.
func (p *StreamParser) Tokens() []uint32 { return append([]uint32(nil), p.tokens...) }

// StateJSON exposes the current state for interop/debugging.
func (p *StreamParser) StateJSON() (string, error) {
	state := struct {
		State string `json:"state"`
	}{State: map[streamState]string{stExpectStart: "ExpectStart", stHeader: "Header", stContent: "Content"}[p.state]}
	b, err := json.Marshal(state)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// CurrentRole returns the role of the current message if known, otherwise the
// next role hint. Nil indicates the role is not yet known.
func (p *StreamParser) CurrentRole() *Role {
	switch p.state {
	case stContent:
		if len(p.messages) == 0 {
			return nil
		}
		r := p.messages[len(p.messages)-1].Author.Role
		return &r
	default:
		return p.nextRole
	}
}

// CurrentContent returns the textual content accumulated so far for the
// current message. Returns an empty string if no content is in progress.
func (p *StreamParser) CurrentContent() string {
	if p.state != stContent {
		return ""
	}
	if len(p.undecodedBytes) == 0 {
		return string(p.contentBytes)
	}
	buf := make([]byte, 0, len(p.contentBytes)+len(p.undecodedBytes))
	buf = append(buf, p.contentBytes...)
	buf = append(buf, []byte(string(bytes.Runes(p.undecodedBytes)))...)
	return string(buf)
}

// CurrentContentType returns the content-type marker (e.g., "<|constrain|>json")
// for the current message if known.
func (p *StreamParser) CurrentContentType() string {
	if p.state != stContent || len(p.messages) == 0 {
		return ""
	}
	return p.messages[len(p.messages)-1].ContentType
}

// CurrentChannel returns the channel for the current message if known.
func (p *StreamParser) CurrentChannel() string {
	if p.state != stContent || len(p.messages) == 0 {
		return ""
	}
	return p.messages[len(p.messages)-1].Channel
}

// CurrentRecipient returns the recipient for the current message if known.
func (p *StreamParser) CurrentRecipient() string {
	if p.state != stContent || len(p.messages) == 0 {
		return ""
	}
	return p.messages[len(p.messages)-1].Recipient
}

// LastContentDelta returns the most recent decoded fragment since the last
// Process call, if any.
func (p *StreamParser) LastContentDelta() string { return string(p.lastDeltaBytes) }

func (p *StreamParser) parseHeaderFromTokens(header []uint32) (parsedHeader, string, error) {
	var hdr parsedHeader
	// decode utf8
	s, err := p.enc.bpe.DecodeUTF8(header)
	if err != nil {
		return hdr, "", err
	}
	s = normalizeHeader(s)
	roleToken, remainder := splitLeadingToken(s)

	detectedRole, nameFromHeader := detectRoleAndAuthor(roleToken, remainder)

	hdr.author.Role = detectedRole
	hdr.author.Name = nameFromHeader
	if p.nextRole != nil {
		hdr.author.Role = *p.nextRole
		if hdr.author.Role == RoleTool && hdr.author.Name == "" {
			hdr.author.Name = nameFromHeader
		}
	}
	// channel
	hdr.channel = extractChannel(s)
	// recipient
	hdr.recipient = extractRecipient(s)
	// content type: remove known parts and trim
	if ct := scrubContentType(roleToken, remainder); ct != "" {
		hdr.contentType = ct
	}
	remaining := headerRemainder(roleToken, remainder, hdr)
	if remaining != "" && p.options.Strict {
		return hdr, "", errors.New("unexpected tokens remaining in message header")
	}
	return hdr, remaining, nil
}

func headerRemainder(roleToken, remainder string, hdr parsedHeader) string {
	s := strings.TrimSpace(remainder)
	if s == "" {
		return ""
	}
	if hdr.channel != "" {
		s = strings.ReplaceAll(s, "<|channel|>"+hdr.channel, "")
		s = strings.ReplaceAll(s, " <|channel|>"+hdr.channel, "")
	}
	if hdr.recipient != "" {
		s = strings.ReplaceAll(s, "to="+hdr.recipient, "")
		s = strings.ReplaceAll(s, " to="+hdr.recipient, "")
		if hdr.author.Role == RoleTool && roleToken == hdr.recipient {
			s = strings.TrimSpace(s)
		}
	}
	if hdr.contentType != "" {
		s = strings.ReplaceAll(s, hdr.contentType, "")
		s = strings.ReplaceAll(s, " "+hdr.contentType, "")
	}
	s = strings.TrimSpace(s)
	return s
}

func (p *StreamParser) renderRecoveredText(text string) {
	if text == "" {
		return
	}
	p.contentBytes = append(p.contentBytes, text...)
	p.lastDeltaBytes = append(p.lastDeltaBytes[:0], text...)
}

func (p *StreamParser) consumeContentToken(token uint32) error {
	p.scratch = p.scratch[:0]
	if err := p.enc.bpe.DecodeTokenBytesInto(&p.scratch, token); err != nil {
		return err
	}
	if len(p.undecodedBytes) == 0 && utf8.Valid(p.scratch) {
		p.contentBytes = append(p.contentBytes, p.scratch...)
		p.lastDeltaBytes = append(p.lastDeltaBytes[:0], p.scratch...)
		return nil
	}

	p.undecodedBytes = append(p.undecodedBytes, p.scratch...)
	p.lastDeltaBytes = p.lastDeltaBytes[:0]
	src := p.undecodedBytes
	i := 0
	for i < len(src) {
		if !utf8.FullRune(src[i:]) {
			break
		}
		r, size := utf8.DecodeRune(src[i:])
		if r == utf8.RuneError && size == 1 {
			i++
			for i < len(src) {
				if !utf8.FullRune(src[i:]) {
					break
				}
				r2, size2 := utf8.DecodeRune(src[i:])
				if r2 != utf8.RuneError || size2 != 1 {
					break
				}
				i++
			}

			p.lastDeltaBytes = utf8.AppendRune(p.lastDeltaBytes, utf8.RuneError)
			p.contentBytes = utf8.AppendRune(p.contentBytes, utf8.RuneError)
			continue
		}
		chunk := src[i : i+size]
		p.lastDeltaBytes = append(p.lastDeltaBytes, chunk...)
		p.contentBytes = append(p.contentBytes, chunk...)
		i += size
	}
	if i > 0 {
		p.undecodedBytes = append(p.undecodedBytes[:0], src[i:]...)
	}
	return nil
}

func derefRole(role *Role, fallback Role) Role {
	if role == nil {
		return fallback
	}
	return *role
}

func mustDecodeLossy(enc *Encoding, toks []uint32) string {
	bs, err := enc.DecodeBytes(toks)
	if err != nil {
		return ""
	}
	return string(bytes.Runes(bs))
}
