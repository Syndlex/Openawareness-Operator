//nolint:revive // utils is a standard package name for utilities
package utils

import (
	"bytes"
	"fmt"
	"text/template"
)

// RenderTemplate processes the input string as a Go template with the provided data.
// Uses [[ ]] delimiters instead of {{ }} to avoid conflicts with Alertmanager templates.
// Supports the "default" function for fallback values: [[ .VAR | default "fallback" ]]
// Returns the rendered string or an error if template parsing or execution fails.
func RenderTemplate(templateStr string, data map[string]string) (string, error) {
	// Create template with custom delimiters [[ ]] and custom functions
	tmpl, err := template.New("config").
		Delims("[[", "]]").
		Option("missingkey=zero").
		Funcs(template.FuncMap{
			"default": defaultFunc,
		}).Parse(templateStr)

	if err != nil {
		return "", fmt.Errorf("failed to parse template: %w", err)
	}

	// Execute template
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to execute template: %w", err)
	}

	return buf.String(), nil
}

// defaultFunc provides default value if the piped value is missing or empty.
// In Go templates, the piped value comes as the last argument.
func defaultFunc(defaultValue string, value string) string {
	if value == "" {
		return defaultValue
	}
	return value
}
