package tokenizer

// tokenStore abstracts storage of base token byte sequences.
// Implementations must not let references to internal storage escape.
type tokenStore interface {
	// AppendInto appends the bytes for token id into dst and returns true
	// if the id existed. Returns false when id is unknown.
	AppendInto(dst *[]byte, id uint32) bool
	// Close releases any resources held by the store.
	Close()
}
