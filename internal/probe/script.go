package probe

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

const metaPrefix = "# PROBEX_META:"

// ScriptProber executes an external script as a probe.
// The script declares its metadata via # PROBEX_META: comment lines.
//
// Input contract:
//   - Env: PROBEX_TARGET, PROBEX_CONFIG (JSON), PROBEX_PARAM_<UPPER_KEY>=value
//   - Stdin: config JSON
//
// Output contract:
//   - Stdout: JSON matching probe.Result schema
//   - Exit 0 = success, non-zero = failure (stderr becomes error message)
type ScriptProber struct {
	meta       ProbeMetadata
	scriptPath string
	interp     string // interpreter detected from shebang
}

// NewScriptProber creates a ScriptProber by reading and parsing the script's
// embedded PROBEX_META header.
func NewScriptProber(scriptPath string) (*ScriptProber, error) {
	f, err := os.Open(scriptPath)
	if err != nil {
		return nil, fmt.Errorf("open script: %w", err)
	}
	defer f.Close()

	var metaLines []string
	var shebang string
	scanner := bufio.NewScanner(f)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		if lineNum == 1 && strings.HasPrefix(line, "#!") {
			shebang = line
		}
		if strings.HasPrefix(strings.TrimSpace(line), metaPrefix) {
			content := strings.TrimSpace(line)
			idx := strings.Index(content, metaPrefix)
			metaLines = append(metaLines, content[idx+len(metaPrefix):])
		}
	}

	if len(metaLines) == 0 {
		return nil, fmt.Errorf("no PROBEX_META found in %s", scriptPath)
	}

	metaJSON := strings.Join(metaLines, "\n")
	var meta ProbeMetadata
	if err := json.Unmarshal([]byte(metaJSON), &meta); err != nil {
		return nil, fmt.Errorf("parse PROBEX_META in %s: %w", scriptPath, err)
	}
	if meta.Name == "" {
		return nil, fmt.Errorf("PROBEX_META missing 'name' in %s", scriptPath)
	}
	meta.Kind = ProbeKindScript

	interp := detectInterpreter(shebang, scriptPath)

	return &ScriptProber{
		meta:       meta,
		scriptPath: scriptPath,
		interp:     interp,
	}, nil
}

func (s *ScriptProber) Name() string           { return s.meta.Name }
func (s *ScriptProber) Metadata() ProbeMetadata { return s.meta }

func (s *ScriptProber) Probe(ctx context.Context, target string, config json.RawMessage) (*Result, error) {
	if len(config) == 0 {
		config = json.RawMessage("{}")
	}

	// Build command
	var cmd *exec.Cmd
	if s.interp != "" {
		cmd = exec.CommandContext(ctx, s.interp, s.scriptPath)
	} else {
		cmd = exec.CommandContext(ctx, s.scriptPath)
	}

	// Set environment variables, enriching PATH with common tool locations
	env := os.Environ()
	env = enrichPath(env)
	env = append(env, "PROBEX_TARGET="+target)
	env = append(env, "PROBEX_CONFIG="+string(config))

	// Extract top-level config params as PROBEX_PARAM_<KEY>
	var params map[string]any
	if json.Unmarshal(config, &params) == nil {
		for k, v := range params {
			key := "PROBEX_PARAM_" + strings.ToUpper(strings.ReplaceAll(k, "-", "_"))
			env = append(env, key+"="+fmt.Sprintf("%v", v))
		}
	}
	cmd.Env = env
	cmd.Stdin = bytes.NewReader(config)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	// Parse stdout as Result JSON
	if stdout.Len() > 0 {
		var result Result
		if parseErr := json.Unmarshal(stdout.Bytes(), &result); parseErr == nil {
			if err != nil && !result.Success {
				// Script exited non-zero and reported failure — use its error
				if result.Error == "" {
					result.Error = strings.TrimSpace(stderr.String())
				}
			}
			return &result, nil
		}
	}

	// Stdout not valid JSON or empty
	if err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg == "" {
			errMsg = fmt.Sprintf("script exited with error: %v", err)
		}
		return &Result{Success: false, Error: errMsg}, nil
	}

	return &Result{Success: false, Error: "script produced no valid JSON output"}, nil
}

// enrichPath ensures PATH includes common tool locations that may not be in
// the controller's inherited environment (e.g., when launched as a service).
func enrichPath(env []string) []string {
	extraPaths := []string{"/opt/homebrew/bin", "/usr/local/bin", "/usr/local/sbin"}
	for i, e := range env {
		if strings.HasPrefix(e, "PATH=") {
			current := e[5:]
			for _, p := range extraPaths {
				if !strings.Contains(current, p) {
					current = p + ":" + current
				}
			}
			env[i] = "PATH=" + current
			return env
		}
	}
	// No PATH found, create one
	env = append(env, "PATH="+strings.Join(extraPaths, ":")+
		":/usr/bin:/bin:/usr/sbin:/sbin")
	return env
}

func detectInterpreter(shebang, path string) string {
	if shebang != "" {
		shebang = strings.TrimPrefix(shebang, "#!")
		shebang = strings.TrimSpace(shebang)
		// Handle "#!/usr/bin/env python3" → "python3" via env
		if strings.HasPrefix(shebang, "/usr/bin/env ") {
			return strings.TrimPrefix(shebang, "/usr/bin/env ")
		}
		return shebang
	}
	// Detect from extension
	switch {
	case strings.HasSuffix(path, ".py"):
		return "python3"
	case strings.HasSuffix(path, ".sh"):
		return "bash"
	case strings.HasSuffix(path, ".rb"):
		return "ruby"
	case strings.HasSuffix(path, ".js"):
		return "node"
	}
	return "bash"
}
