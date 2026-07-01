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
