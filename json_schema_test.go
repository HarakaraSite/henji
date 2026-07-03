package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func writeTempSchema(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "schema.json")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}

func TestLoadJSONSchema(t *testing.T) {
	path := writeTempSchema(t, `{"type":"object","properties":{"ok":{"type":"boolean"}},"required":["ok"]}`)

	doc, schema, err := loadJSONSchema(path)
	require.NoError(t, err)
	require.Equal(t, "object", doc["type"])
	require.NoError(t, validateAgainstSchema(schema, `{"ok":true}`))
}

func TestLoadJSONSchemaMissingFile(t *testing.T) {
	_, _, err := loadJSONSchema(filepath.Join(t.TempDir(), "does-not-exist.json"))
	require.Error(t, err)
}

func TestLoadJSONSchemaInvalidJSON(t *testing.T) {
	path := writeTempSchema(t, `{not json`)
	_, _, err := loadJSONSchema(path)
	require.Error(t, err)
}

func TestLoadJSONSchemaInvalidSchema(t *testing.T) {
	// "type" must be a string or array of strings, not a number.
	path := writeTempSchema(t, `{"type": 123}`)
	_, _, err := loadJSONSchema(path)
	require.Error(t, err)
}

func TestValidateAgainstSchema(t *testing.T) {
	_, schema, err := loadJSONSchema(writeTempSchema(t, `{"type":"object","properties":{"ok":{"type":"boolean"}},"required":["ok"]}`))
	require.NoError(t, err)

	require.NoError(t, validateAgainstSchema(schema, `{"ok":true}`))
	require.Error(t, validateAgainstSchema(schema, `{"ok":"not a bool"}`), "schema mismatch must error")
	require.Error(t, validateAgainstSchema(schema, `not json at all`), "unparseable content must error")
}
