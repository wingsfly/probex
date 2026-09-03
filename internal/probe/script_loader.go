package probe

import (
	"log/slog"
	"os"
	"path/filepath"
)

// LoadScripts scans a directory for probe scripts and registers them.
// Files must contain a valid PROBEX_META header to be registered.
// Non-probe files are silently skipped.
func LoadScripts(dir string, registry *Registry, logger *slog.Logger) int {
	if dir == "" {
		return 0
	}
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		logger.Debug("script dir not found, skipping", "dir", dir)
		return 0
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		logger.Error("read script dir", "dir", dir, "error", err)
		return 0
	}

	loaded := 0
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		path := filepath.Join(dir, entry.Name())

		prober, err := NewScriptProber(path)
		if err != nil {
			logger.Debug("skip file (not a probe script)", "file", entry.Name(), "reason", err.Error())
			continue
		}

		// Check for name conflicts
		if _, exists := registry.GetMetadata(prober.Name()); exists {
			logger.Warn("script probe name conflict, skipping", "name", prober.Name(), "file", entry.Name())
			continue
		}

		registry.Register(prober)
		loaded++
		logger.Info("loaded script probe", "name", prober.Name(), "file", entry.Name())
	}

	if loaded > 0 {
		logger.Info("script probes loaded", "count", loaded, "dir", dir)
	}
	return loaded
}

// ReloadScripts clears previously loaded script probes and re-scans the
// directory, so new/edited scripts (and changed PROBEX_META metadata) take
// effect without restarting the process. Returns the number loaded.
func ReloadScripts(dir string, registry *Registry, logger *slog.Logger) int {
	registry.RemoveByKind(ProbeKindScript)
	return LoadScripts(dir, registry, logger)
}
