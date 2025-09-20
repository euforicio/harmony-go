package tokenizer

import "testing"

func TestHeapStoreAppendIntoSmallVocab(t *testing.T) {
	pairs := [][2]any{
		{[]byte("hi"), uint32(1)},
		{[]byte("bye"), uint32(2)},
	}

	store, err := newTokenStore(pairs)
	if err != nil {
		t.Fatalf("newTokenStore: %v", err)
	}
	t.Cleanup(store.Close)

	var dst []byte
	if ok := store.AppendInto(&dst, 1); !ok {
		t.Fatalf("expected id 1 to be present")
	}
	if got := string(dst); got != "hi" {
		t.Fatalf("unexpected bytes after first append: %q", got)
	}
	if ok := store.AppendInto(&dst, 2); !ok {
		t.Fatalf("expected id 2 to be present")
	}
	if got := string(dst); got != "hibye" {
		t.Fatalf("unexpected bytes after second append: %q", got)
	}
	if ok := store.AppendInto(&dst, 3); ok {
		t.Fatalf("unexpected success for missing id")
	}
}
