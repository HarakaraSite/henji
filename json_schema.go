package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// loadJSONSchema reads and compiles the file at path for use with
// --json-schema. It returns the raw schema (sent to providers as-is) and a
// compiled validator (used to check the model's response client-side). Both
// steps fail fast so a bad --json-schema value is caught at startup rather
// than after the model has already answered.
func loadJSONSchema(path string) (map[string]any, *jsonschema.Schema, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("could not read --json-schema file: %w", err)
	}

	var doc map[string]any
	if err := json.Unmarshal(content, &doc); err != nil {
		return nil, nil, fmt.Errorf("--json-schema file is not valid JSON: %w", err)
	}

	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource(path, doc); err != nil {
		return nil, nil, fmt.Errorf("--json-schema file is not a valid JSON Schema: %w", err)
	}
	schema, err := compiler.Compile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("--json-schema file is not a valid JSON Schema: %w", err)
	}

	return doc, schema, nil
}

// validateAgainstSchema checks that content is JSON conforming to schema. A
// content string that isn't even valid JSON is reported the same way as a
// schema mismatch, since both mean the model didn't produce a usable answer.
func validateAgainstSchema(schema *jsonschema.Schema, content string) error {
	var v any
	if err := json.Unmarshal([]byte(content), &v); err != nil {
		return fmt.Errorf("response is not valid JSON: %w", err)
	}
	if err := schema.Validate(v); err != nil {
		return fmt.Errorf("response does not match --json-schema: %w", err)
	}
	return nil
}
