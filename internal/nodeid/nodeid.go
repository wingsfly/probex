// Package nodeid provides a persistent, unique 8-character hex node identifier.
//
// The ID is generated once and stored in a file on disk so that it survives
// restarts. Every process on the same machine that shares the same data
// directory will resolve to the same node ID, enabling the ProbeX server to
// correlate results even when user-chosen names collide.
//
// Default storage path: ~/.probex/node_id
package nodeid

import (
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const (
	// IDBytes is the number of random bytes (8 hex chars = 4 bytes).
	IDBytes  = 4
	fileName = "node_id"
)

var (
	cached string
	mu     sync.Mutex
)

// Get returns the persistent node ID, generating one if it does not exist.
// dataDir is the directory to store the ID file; if empty, ~/.probex is used.
func Get(dataDir string) (string, error) {
	mu.Lock()
	defer mu.Unlock()

	if cached != "" {
		return cached, nil
	}

	dir := dataDir
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("nodeid: cannot determine home dir: %w", err)
		}
		dir = filepath.Join(home, ".probex")
	}

	path := filepath.Join(dir, fileName)

	// Try to read existing ID
	if data, err := os.ReadFile(path); err == nil {
		id := strings.TrimSpace(string(data))
		if len(id) == IDBytes*2 && isHex(id) {
			cached = id
			return cached, nil
		}
		// Invalid content — regenerate
	}

	// Generate new ID
	b := make([]byte, IDBytes)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("nodeid: rand failed: %w", err)
	}
	id := fmt.Sprintf("%x", b)

	// Persist
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("nodeid: mkdir %s: %w", dir, err)
	}
	if err := os.WriteFile(path, []byte(id+"\n"), 0o644); err != nil {
		return "", fmt.Errorf("nodeid: write %s: %w", path, err)
	}

	cached = id
	return cached, nil
}

// Reset clears the in-memory cache (useful for testing).
func Reset() {
	mu.Lock()
	cached = ""
	mu.Unlock()
}

func isHex(s string) bool {
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return false
		}
	}
	return true
}
