package tokenizer

import (
	"bufio"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	// Matches the upstream default base URL used across Harmony implementations.
	defaultBaseURL = "https://openaipublic.blob.core.windows.net/encodings/"
	envEncBase     = "TIKTOKEN_ENCODINGS_BASE"
	envCacheDir    = "TIKTOKEN_GO_CACHE_DIR"
	envOffline     = "TIKTOKEN_OFFLINE"
	envHTTPTimeout = "TIKTOKEN_HTTP_TIMEOUT" // seconds
	expectedO200k  = "446a9538cb6c348e3516120d7c08b09f57c36495e2acfffe59a5bf8b0cfb1a2d"
)

// resolveCacheDir respects the Go-specific cache override or falls back to a predictable temp directory.
func resolveCacheDir() (string, error) {
	if d := os.Getenv(envCacheDir); d != "" {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return "", err
		}
		return d, nil
	}
	primary := filepath.Join(os.TempDir(), "tiktoken-go-cache")
	if err := os.MkdirAll(primary, 0o755); err != nil {
		return "", err
	}
	return primary, nil
}

func baseURL() string {
	base := os.Getenv(envEncBase)
	if base == "" {
		return defaultBaseURL
	}
	if !strings.HasSuffix(base, "/") {
		base += "/"
	}
	return base
}

func downloadToFile(url, dest string) (string, error) {
	// Bounded HTTP client to avoid indefinite hangs in restricted environments.
	timeout := 30 * time.Second
	if v := os.Getenv(envHTTPTimeout); v != "" {
		if s, err := strconv.Atoi(v); err == nil && s > 0 {
			timeout = time.Duration(s) * time.Second
		}
	}
	client := &http.Client{Timeout: timeout}
	resp, err := client.Get(url)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("unexpected status %s", resp.Status)
	}
	f, err := os.Create(dest)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	mw := io.MultiWriter(f, h)
	if _, err := io.Copy(mw, resp.Body); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

// LoadO200k reads or downloads o200k_base.tiktoken and returns encoder pairs.
// Each line: base64_token + space + rank.
func LoadO200k() (pairs [][2]interface{}, err error) {
	// Resolve file path
	var path string
	if b := os.Getenv(envEncBase); b != "" {
		// treat as directory
		path = filepath.Join(b, "o200k_base.tiktoken")
	} else {
		cacheDir, e := resolveCacheDir()
		if e != nil {
			return nil, e
		}
		path = filepath.Join(cacheDir, "o200k_base.tiktoken")
		if _, e := os.Stat(path); errors.Is(e, os.ErrNotExist) {
			if os.Getenv(envOffline) == "1" {
				return nil, fmt.Errorf("o200k file missing and TIKTOKEN_OFFLINE=1; set %s to local dir containing o200k_base.tiktoken or unset offline", envEncBase)
			}
			url := baseURL() + "o200k_base.tiktoken"
			sum, e := downloadToFile(url, path)
			if e != nil {
				return nil, e
			}
			if !strings.EqualFold(sum, expectedO200k) {
				return nil, fmt.Errorf("hash mismatch: got %s want %s", sum, expectedO200k)
			}
		}
	}

	f, e := os.Open(path)
	if e != nil {
		return nil, e
	}
	defer func() { _ = f.Close() }()
	r := bufio.NewReader(f)
	lineNo := 0
	for {
		line, e := r.ReadString('\n')
		if e != nil && !errors.Is(e, io.EOF) {
			return nil, e
		}
		if line == "" && errors.Is(e, io.EOF) {
			break
		}
		lineNo++
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			continue
		}
		sp := strings.IndexByte(line, ' ')
		if sp <= 0 {
			return nil, fmt.Errorf("invalid vocab at line %d", lineNo)
		}
		b64 := line[:sp]
		rankStr := line[sp+1:]
		tok, de := base64.StdEncoding.DecodeString(b64)
		if de != nil {
			return nil, fmt.Errorf("b64 decode line %d: %w", lineNo, de)
		}
		// parse rank (uint32) — use strconv to avoid fmt scanning allocations
		rank, se := strconv.ParseUint(rankStr, 10, 32)
		if se != nil {
			return nil, fmt.Errorf("rank parse line %d: %w", lineNo, se)
		}
		pairs = append(pairs, [2]interface{}{tok, uint32(rank)})
		if errors.Is(e, io.EOF) {
			break
		}
	}
	return pairs, nil
}
