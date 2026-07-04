package main

import (
	"os"
	"path/filepath"
	"runtime"
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

func TestEnsureConfigSamplingValues(t *testing.T) {
	writeConfig := func(t *testing.T, content string) {
		t.Helper()
		configHome := t.TempDir()
		dataHome := t.TempDir()
		t.Setenv("XDG_CONFIG_HOME", configHome)
		t.Setenv("XDG_DATA_HOME", dataHome)

		dir := filepath.Join(configHome, "henji")
		require.NoError(t, os.MkdirAll(dir, 0o700))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "henji.yml"), []byte(content), 0o600))
	}

	t.Run("omitted values remain disabled", func(t *testing.T) {
		writeConfig(t, "{}\n")

		cfg, err := ensureConfig()
		require.NoError(t, err)
		require.Equal(t, -1.0, cfg.Temperature)
		require.Equal(t, -1.0, cfg.TopP)
		require.Equal(t, int64(-1), cfg.TopK)
	})

	t.Run("explicit zero values are preserved", func(t *testing.T) {
		writeConfig(t, "temp: 0\ntopp: 0\ntopk: 0\n")

		cfg, err := ensureConfig()
		require.NoError(t, err)
		require.Zero(t, cfg.Temperature)
		require.Zero(t, cfg.TopP)
		require.Zero(t, cfg.TopK)
	})

	t.Run("environment can explicitly set zero", func(t *testing.T) {
		writeConfig(t, "{}\n")
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
