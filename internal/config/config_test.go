package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func env(vars map[string]string) func(string) (string, bool) {
	return func(name string) (string, bool) {
		v, ok := vars[name]
		return v, ok
	}
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestLoad_Precedence(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeFile(t, dir, SecretsFile, "CODEPEN_API_KEY=from-env-file\nCODEPEN_DIR=secrets-dir\nCODEPEN_API_URL=http://secrets\n")
	writeFile(t, dir, StateFile, "CODEPEN_DIR=pen\nCODEPEN_API_URL=http://state\n")
	cfg, err := Load(dir, env(map[string]string{"CODEPEN_API_URL": "http://process", "CODEPEN_API_KEY": ""}))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.APIKey != "from-env-file" {
		t.Errorf("APIKey = %q (an empty process value must not win)", cfg.APIKey)
	}
	if cfg.Dir != filepath.Join(dir, "pen") {
		t.Errorf("Dir = %q, want state file's pen", cfg.Dir)
	}
	if cfg.APIURL != "http://process" {
		t.Errorf("APIURL = %q, want the process env", cfg.APIURL)
	}
}

// The key goes wherever CODEPEN_API_URL points, so a cloned repo's env files
// must not be able to redirect it.
func TestLoad_APIURLOnlyFromProcessEnv(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeFile(t, dir, SecretsFile, "CODEPEN_API_URL=https://evil.example\n")
	writeFile(t, dir, StateFile, "CODEPEN_API_URL=https://evil.example\n")
	cfg, err := Load(dir, env(nil))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.APIURL != "" {
		t.Errorf("APIURL = %q, want the production API", cfg.APIURL)
	}
}

func TestLoad_Defaults(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	cfg, err := Load(dir, env(nil))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Dir != dir || cfg.PenID != "" || cfg.PollInterval != 0 || cfg.APIURL != "" {
		t.Errorf("Load() = %+v", cfg)
	}
	if _, err := cfg.Key(); err == nil || !strings.Contains(err.Error(), "CODEPEN_API_KEY is not set") {
		t.Errorf("Key() error = %v", err)
	}
}

func TestLoad_Values(t *testing.T) {
	t.Parallel()
	cfg, err := Load(t.TempDir(), env(map[string]string{
		"CODEPEN_ID":      "https://codepen.io/me/pen/PwWmgeN",
		"CODEPEN_POLL_MS": "5",
		"CODEPEN_DIR":     "/abs/dir",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.PenID != "PwWmgeN" || cfg.PollInterval != 5*time.Millisecond || cfg.Dir != "/abs/dir" {
		t.Errorf("Load() = %+v", cfg)
	}
}

func TestLoad_Invalid(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		vars    map[string]string
		message string
	}{
		{name: "bad pen id", vars: map[string]string{"CODEPEN_ID": "not an id"}, message: "CODEPEN_ID"},
		{name: "bad poll interval", vars: map[string]string{"CODEPEN_POLL_MS": "soon"}, message: "CODEPEN_POLL_MS"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := Load(t.TempDir(), env(tt.vars))
			if err == nil || !strings.Contains(err.Error(), tt.message) {
				t.Errorf("Load() error = %v, want %s", err, tt.message)
			}
		})
	}
}

func TestLoad_EmptyFileValueFallsThrough(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeFile(t, dir, SecretsFile, "CODEPEN_API_KEY=real\n")
	writeFile(t, dir, StateFile, "CODEPEN_API_KEY=\n")
	cfg, err := Load(dir, env(nil))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.APIKey != "real" {
		t.Errorf("APIKey = %q, want the .env value", cfg.APIKey)
	}
}

func TestLoad_RelativeDir(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	cfg, err := Load(dir, env(map[string]string{"CODEPEN_DIR": "pen"}))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Dir != filepath.Join(dir, "pen") {
		t.Errorf("Dir = %q", cfg.Dir)
	}
}

func TestKey_ReturnsKey(t *testing.T) {
	t.Parallel()
	cfg, err := Load(t.TempDir(), env(map[string]string{"CODEPEN_API_KEY": "cp_user_abc"}))
	if err != nil {
		t.Fatal(err)
	}
	if got, err := cfg.Key(); err != nil || got != "cp_user_abc" {
		t.Errorf("Key() = %q, %v", got, err)
	}
}

func TestKey_RejectsOnePasswordReference(t *testing.T) {
	t.Parallel()
	cfg, err := Load(t.TempDir(), env(map[string]string{"CODEPEN_API_KEY": "op://Private/CodePen/API Key"}))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cfg.Key(); err == nil || !strings.Contains(err.Error(), "op run") {
		t.Errorf("Key() error = %v, want guidance to resolve it", err)
	}
}

func TestLoad_PenIDFromPenConfig(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		vars     map[string]string
		expected string
	}{
		{name: "from the config file", vars: nil, expected: "PwWmgeN"},
		{name: "env wins", vars: map[string]string{"CODEPEN_ID": "AbCdEfG"}, expected: "AbCdEfG"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			if err := os.MkdirAll(filepath.Join(dir, "pen"), 0o755); err != nil {
				t.Fatal(err)
			}
			writeFile(t, filepath.Join(dir, "pen"), ".codepen.config.json", `{"id":"PwWmgeN"}`)
			vars := map[string]string{"CODEPEN_DIR": "pen"}
			for k, v := range tt.vars {
				vars[k] = v
			}
			cfg, err := Load(dir, env(vars))
			if err != nil || cfg.PenID != tt.expected {
				t.Errorf("Load() PenID = %q, %v; want %q", cfg.PenID, err, tt.expected)
			}
		})
	}
}
