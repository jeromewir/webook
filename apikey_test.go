package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func createTestAPIKey(t *testing.T, name string) string {
	t.Helper()
	var output bytes.Buffer
	if err := runAPIKeyCLI([]string{"create", "--name", name}, &output); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	return lines[len(lines)-1]
}

func TestAPIKeyCreation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "keys.json")
	t.Setenv("API_KEYS_FILE", path)
	first := createTestAPIKey(t, "first")
	second := createTestAPIKey(t, "second")
	if first == second || len(first) != 43 {
		t.Fatal("expected distinct 256-bit keys")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(data, []byte(first)) || bytes.Contains(data, []byte(second)) {
		t.Fatal("key file contains plaintext secrets")
	}
	keys, err := readAPIKeys(path)
	if err != nil || len(keys) != 2 {
		t.Fatalf("expected both keys to be preserved: %v", err)
	}
	hash := sha256.Sum256([]byte(first))
	if keys[0].Hash != hex.EncodeToString(hash[:]) || keys[0].CreatedAt.IsZero() {
		t.Fatal("expected SHA-256 hash and creation timestamp")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("permissions = %o, want 600", info.Mode().Perm())
	}
	if err := runAPIKeyCLI([]string{"create", "--name", "first"}, &bytes.Buffer{}); err == nil {
		t.Fatal("expected duplicate name to be rejected")
	}
}

func TestAPIKeyProtectionAndReload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "keys.json")
	t.Setenv("API_KEYS_FILE", path)
	first := createTestAPIKey(t, "first")
	store, err := newAPIKeyStore(path)
	if err != nil {
		t.Fatal(err)
	}
	handler := store.protect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	check := func(header string, want int) {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, "/api/book", nil)
		req.Header.Set("Authorization", header)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, req)
		if response.Code != want {
			t.Fatalf("response = %d, want %d", response.Code, want)
		}
		if want == http.StatusUnauthorized && response.Header().Get("WWW-Authenticate") != "Bearer" {
			t.Fatal("missing bearer challenge")
		}
	}
	for _, header := range []string{"", "Bearer", "Basic abc", "Bearer unknown", "Bearer a b"} {
		check(header, http.StatusUnauthorized)
	}
	check("Bearer "+first, http.StatusNoContent)
	second := createTestAPIKey(t, "second")
	check("Bearer "+second, http.StatusNoContent)
	if err := os.WriteFile(path, []byte("invalid json"), 0600); err != nil {
		t.Fatal(err)
	}
	check("Bearer unknown", http.StatusUnauthorized)
	check("Bearer "+second, http.StatusNoContent)
	check("bearer "+first, http.StatusNoContent)
	if err := writeAPIKeys(path, nil); err != nil {
		t.Fatal(err)
	}
	check("Bearer unknown", http.StatusUnauthorized)
	check("Bearer "+first, http.StatusUnauthorized)
}

func TestAPIKeyStartupValidation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "keys.json")
	if _, err := newAPIKeyStore(path); err == nil {
		t.Fatal("expected missing file to prevent startup")
	}
	for _, data := range []string{"[]", "null", "invalid", `[{"name":"bad","hash":"xyz"}]`} {
		if err := os.WriteFile(path, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := newAPIKeyStore(path); err == nil {
			t.Fatalf("expected startup failure for %q", data)
		}
	}
}
