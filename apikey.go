package main

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type apiKeyRecord struct {
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"createdAt"`
	Hash      string    `json:"hash"`
}

func apiKeysPath() string {
	if path := os.Getenv("API_KEYS_FILE"); path != "" {
		return path
	}
	return "api-keys.json"
}

func readAPIKeys(path string) ([]apiKeyRecord, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var keys []apiKeyRecord
	if err := json.Unmarshal(data, &keys); err != nil {
		return nil, fmt.Errorf("invalid API key file: %w", err)
	}
	for _, key := range keys {
		hash, err := hex.DecodeString(key.Hash)
		if err != nil || len(hash) != sha256.Size || strings.TrimSpace(key.Name) == "" {
			return nil, errors.New("invalid API key record: expected a name and SHA-256 hash")
		}
	}
	return keys, nil
}

// Write via rename so the server never reads a partially written key file.
func writeAPIKeys(path string, keys []apiKeyRecord) error {
	data, err := json.MarshalIndent(keys, "", "  ")
	if err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".api-keys-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if _, err := file.Write(append(data, '\n')); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(file.Name(), path)
}

func runAPIKeyCLI(args []string, out io.Writer) error {
	if len(args) == 0 || args[0] != "create" {
		return errors.New("usage: webook apikey create --name <name>")
	}
	flags := flag.NewFlagSet("apikey create", flag.ContinueOnError)
	flags.SetOutput(out)
	name := flags.String("name", "", "name identifying this API key")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if strings.TrimSpace(*name) == "" || flags.NArg() != 0 {
		return errors.New("usage: webook apikey create --name <name>")
	}
	path := apiKeysPath()
	keys, err := readAPIKeys(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	for _, key := range keys {
		if key.Name == *name {
			return fmt.Errorf("API key named %q already exists", *name)
		}
	}
	secret, err := randomURLString(32)
	if err != nil {
		return err
	}
	hash := sha256.Sum256([]byte(secret))
	keys = append(keys, apiKeyRecord{Name: *name, CreatedAt: time.Now().UTC(), Hash: hex.EncodeToString(hash[:])})
	if err := writeAPIKeys(path, keys); err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, "API key created. Save it now; it cannot be shown again:\n%s\n", secret)
	return err
}

type apiKeyStore struct {
	mu     sync.Mutex
	path   string
	hashes [][sha256.Size]byte
}

func newAPIKeyStore(path string) (*apiKeyStore, error) {
	store := &apiKeyStore{path: path}
	if err := store.reload(); err != nil {
		return nil, err
	}
	if len(store.hashes) == 0 {
		return nil, errors.New("API key file is empty; create a key with: webook apikey create --name <name>")
	}
	return store, nil
}

// Caller holds mu once the store is shared with HTTP handlers.
func (s *apiKeyStore) reload() error {
	keys, err := readAPIKeys(s.path)
	if err != nil {
		return err
	}
	hashes := make([][sha256.Size]byte, 0, len(keys))
	for _, key := range keys {
		decoded, _ := hex.DecodeString(key.Hash) // Validated by readAPIKeys.
		var hash [sha256.Size]byte
		copy(hash[:], decoded)
		hashes = append(hashes, hash)
	}
	s.hashes = hashes
	return nil
}

func (s *apiKeyStore) matches(hash [sha256.Size]byte) bool {
	match := 0
	for _, stored := range s.hashes {
		match |= subtle.ConstantTimeCompare(hash[:], stored[:])
	}
	return match == 1
}

func (s *apiKeyStore) valid(secret string) bool {
	hash := sha256.Sum256([]byte(secret))
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.matches(hash) {
		return true
	}
	return s.reload() == nil && s.matches(hash)
}

func (s *apiKeyStore) protect(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Fields(r.Header.Get("Authorization"))
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || !s.valid(parts[1]) {
			w.Header().Set("WWW-Authenticate", "Bearer")
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}
