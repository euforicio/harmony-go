package tokenizer

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoaderOfflineMissingCacheFailsFast(t *testing.T) {
	t.Setenv(envOffline, "1")
	cacheDir := t.TempDir()
	t.Setenv(envCacheDir, cacheDir)
	t.Setenv(envEncBase, "")

	_, err := LoadO200k()
	if err == nil {
		t.Fatalf("expected error when offline cache is missing")
	}
	if !strings.Contains(err.Error(), "TIKTOKEN_OFFLINE") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoaderDownloadTimeout(t *testing.T) {
	t.Setenv(envHTTPTimeout, "1")

	dest := filepath.Join(t.TempDir(), "out")
	start := time.Now()
	if _, err := downloadToFile("http://10.255.255.1:81", dest); err == nil {
		t.Fatalf("expected timeout error")
	} else {
		if elapsed := time.Since(start); elapsed > 5*time.Second {
			t.Fatalf("download exceeded expected timeout: %v", elapsed)
		}
	}
}
