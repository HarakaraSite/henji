package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestConfig(t *testing.T) {
	t.Run("old format text", func(t *testing.T) {
		var cfg Config
		require.NoError(t, yaml.Unmarshal([]byte("format-text: as markdown"), &cfg))
		require.Equal(t, FormatText(map[string]string{
			"markdown": "as markdown",
		}), cfg.FormatText)
	})
	t.Run("new format text", func(t *testing.T) {
		var cfg Config
		require.NoError(t, yaml.Unmarshal([]byte("format-text:\n  markdown: as markdown\n  json: as json"), &cfg))
		require.Equal(t, FormatText(map[string]string{
			"markdown": "as markdown",
			"json":     "as json",
		}), cfg.FormatText)
	})
}

func writeTestConfig(t *testing.T, content string) {
	t.Helper()
	for _, entry := range os.Environ() {
		key, _, ok := strings.Cut(entry, "=")
		if !ok || !strings.HasPrefix(key, "HENJI_") {
			continue
		}
		value := os.Getenv(key)
		require.NoError(t, os.Unsetenv(key))
		t.Cleanup(func() { require.NoError(t, os.Setenv(key, value)) })
	}

	configHome := t.TempDir()
	dataHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)
	t.Setenv("XDG_DATA_HOME", dataHome)

	dir := filepath.Join(configHome, "henji")
	require.NoError(t, os.MkdirAll(dir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "henji.yml"), []byte(content), 0o600))
}

func TestEnsureConfigSamplingValues(t *testing.T) {
	t.Run("omitted values remain disabled", func(t *testing.T) {
		writeTestConfig(t, "{}\n")

		cfg, err := ensureConfig()
		require.NoError(t, err)
		require.Equal(t, -1.0, cfg.Temperature)
		require.Equal(t, -1.0, cfg.TopP)
		require.Equal(t, int64(-1), cfg.TopK)
	})

	t.Run("explicit zero values are preserved", func(t *testing.T) {
		writeTestConfig(t, "temp: 0\ntopp: 0\ntopk: 0\n")

		cfg, err := ensureConfig()
		require.NoError(t, err)
		require.Zero(t, cfg.Temperature)
		require.Zero(t, cfg.TopP)
		require.Zero(t, cfg.TopK)
	})

	t.Run("environment can explicitly set zero", func(t *testing.T) {
		writeTestConfig(t, "{}\n")
		t.Setenv("HENJI_TEMP", "0")
		t.Setenv("HENJI_TOPP", "0")
		t.Setenv("HENJI_TOPK", "0")

		cfg, err := ensureConfig()
		require.NoError(t, err)
		require.Zero(t, cfg.Temperature)
		require.Zero(t, cfg.TopP)
		require.Zero(t, cfg.TopK)
	})
}

// TestEnsureConfigDefaultMaxRetries is the F-4 regression test: a minimal
// hand-written henji.yml (without max-retries: 5, unlike the bundled
// config_template.yml) used to leave MaxRetries at 0, causing m.retry() to
// bail out on the very first error instead of retrying or falling back to
// another model on a 404.
func TestEnsureConfigDefaultMaxRetries(t *testing.T) {
	writeTestConfig(t, "{}\n")

	cfg, err := ensureConfig()
	require.NoError(t, err)
	require.Equal(t, 5, cfg.MaxRetries)
}

// TestEnsureConfigRepairsLoosePermissions is the HENJI-SEC-003 regression
// test: a settings file inherited from a pre-v2 install (or otherwise
// created with a loose mode) must have its permissions restricted to 0600
// before ensureConfig reads the plaintext api-key it may contain.
func TestEnsureConfigRepairsLoosePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission bits are not meaningful on Windows; ACL handling is unimplemented")
	}

	writeTestConfig(t, "{}\n")
	path := filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "henji", "henji.yml")
	require.NoError(t, os.Chmod(path, 0o644))

	_, err := ensureConfig()
	require.NoError(t, err)

	info, err := os.Stat(path)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

func TestEnsureConfigEnvironmentOverrides(t *testing.T) {
	t.Run("parses supported scalar and slice types", func(t *testing.T) {
		writeTestConfig(t, "{}\n")
		t.Setenv("HENJI_FORMAT", "true")
		t.Setenv("HENJI_MAX_TOKENS", "2048")
		t.Setenv("HENJI_TEMP", "0.25")
		t.Setenv("HENJI_STOP", "END,DONE")

		cfg, err := ensureConfig()
		require.NoError(t, err)
		require.True(t, cfg.Format)
		require.Equal(t, int64(2048), cfg.MaxTokens)
		require.Equal(t, 0.25, cfg.Temperature)
		require.Equal(t, []string{"END", "DONE"}, cfg.Stop)
	})

	t.Run("rejects an invalid value", func(t *testing.T) {
		writeTestConfig(t, "{}\n")
		t.Setenv("HENJI_MAX_TOKENS", "not-an-integer")

		_, err := ensureConfig()
		require.Error(t, err)
		require.Contains(t, err.Error(), "MaxTokens")
		require.Contains(t, err.Error(), "invalid syntax")
	})
}

func TestConfigHomeDir(t *testing.T) {
	t.Run("respects XDG_CONFIG_HOME", func(t *testing.T) {
		t.Setenv("XDG_CONFIG_HOME", "/custom/config")
		got, err := configHomeDir()
		require.NoError(t, err)
		require.Equal(t, "/custom/config", got)
	})

	t.Run("defaults to a single OS-specific path when unset", func(t *testing.T) {
		t.Setenv("XDG_CONFIG_HOME", "")
		home, err := os.UserHomeDir()
		require.NoError(t, err)

		got, err := configHomeDir()
		require.NoError(t, err)

		switch runtime.GOOS {
		case "windows":
			require.NotEmpty(t, got)
		default:
			require.Equal(t, filepath.Join(home, ".config"), got)
		}
	})
}

func TestDataHomeDir(t *testing.T) {
	t.Run("respects XDG_DATA_HOME", func(t *testing.T) {
		t.Setenv("XDG_DATA_HOME", "/custom/data")
		got, err := dataHomeDir()
		require.NoError(t, err)
		require.Equal(t, "/custom/data", got)
	})

	t.Run("defaults to a single OS-specific path when unset", func(t *testing.T) {
		t.Setenv("XDG_DATA_HOME", "")
		home, err := os.UserHomeDir()
		require.NoError(t, err)

		got, err := dataHomeDir()
		require.NoError(t, err)

		switch runtime.GOOS {
		case "windows":
			require.NotEmpty(t, got)
		default:
			require.Equal(t, filepath.Join(home, ".local", "share"), got)
		}
	})
}
