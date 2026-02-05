package utils

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestTemplate(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Template Utility Suite")
}

var _ = Describe("RenderTemplate", func() {
	Context("Basic variable substitution", func() {
		It("should substitute a single variable", func() {
			template := "Hello [[ .NAME ]]"
			data := map[string]string{"NAME": "World"}

			result, err := RenderTemplate(template, data)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal("Hello World"))
		})

		It("should substitute multiple variables", func() {
			template := "[[ .GREETING ]] [[ .NAME ]]!"
			data := map[string]string{
				"GREETING": "Hello",
				"NAME":     "Kubernetes",
			}

			result, err := RenderTemplate(template, data)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal("Hello Kubernetes!"))
		})

		It("should handle variables in YAML-like content", func() {
			template := `global:
  smtp_smarthost: '[[ .SMTP_HOST ]]'
  smtp_from: '[[ .SMTP_FROM ]]'`
			data := map[string]string{
				"SMTP_HOST": "mail.example.com:587",
				"SMTP_FROM": "alerts@example.com",
			}

			result, err := RenderTemplate(template, data)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring("smtp_smarthost: 'mail.example.com:587'"))
			Expect(result).To(ContainSubstring("smtp_from: 'alerts@example.com'"))
		})
	})

	Context("Default function", func() {
		It("should use provided value when variable exists", func() {
			template := "Value: [[ .KEY | default \"fallback\" ]]"
			data := map[string]string{"KEY": "actual"}

			result, err := RenderTemplate(template, data)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal("Value: actual"))
		})

		It("should use default value when variable is missing", func() {
			template := "Value: [[ .MISSING | default \"fallback\" ]]"
			data := map[string]string{}

			result, err := RenderTemplate(template, data)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal("Value: fallback"))
		})

		It("should use default value when variable is empty string", func() {
			template := "Value: [[ .EMPTY | default \"fallback\" ]]"
			data := map[string]string{"EMPTY": ""}

			result, err := RenderTemplate(template, data)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal("Value: fallback"))
		})

		It("should handle multiple default functions", func() {
			template := "[[ .A | default \"a\" ]] [[ .B | default \"b\" ]]"
			data := map[string]string{"A": "first"}

			result, err := RenderTemplate(template, data)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal("first b"))
		})
	})

	Context("Error handling", func() {
		It("should return error for invalid template syntax", func() {
			template := "[[ .VAR" // Missing closing braces
			data := map[string]string{"VAR": "value"}

			_, err := RenderTemplate(template, data)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to parse template"))
		})

		It("should return empty string for undefined variable without default", func() {
			// With missingkey=zero option, undefined variables return empty string
			// This is acceptable - users should use the default function for required variables
			template := "Value: [[ .UNDEFINED ]]"
			data := map[string]string{}

			result, err := RenderTemplate(template, data)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal("Value: "))
		})

		It("should return error for invalid template action", func() {
			template := "[[ .VAR.Invalid ]]"
			data := map[string]string{"VAR": "value"}

			_, err := RenderTemplate(template, data)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to execute template"))
		})
	})

	Context("Edge cases", func() {
		It("should handle empty template string", func() {
			template := ""
			data := map[string]string{"VAR": "value"}

			result, err := RenderTemplate(template, data)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(""))
		})

		It("should handle nil data map", func() {
			template := "No variables here"

			result, err := RenderTemplate(template, nil)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal("No variables here"))
		})

		It("should handle template with no variables", func() {
			template := "Static content only"
			data := map[string]string{"UNUSED": "value"}

			result, err := RenderTemplate(template, data)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal("Static content only"))
		})

		It("should handle special characters in values", func() {
			template := "Special: [[ .CHARS ]]"
			data := map[string]string{
				"CHARS": "!@#$%^&*()_+-={}[]|\\:\";<>?,./",
			}

			result, err := RenderTemplate(template, data)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring("!@#$%^&*()_+-={}[]|\\:\";<>?,./"))
		})

		It("should handle newlines in values", func() {
			template := "Multi:\n[[ .TEXT ]]"
			data := map[string]string{
				"TEXT": "line1\nline2\nline3",
			}

			result, err := RenderTemplate(template, data)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring("line1\nline2\nline3"))
		})

		It("should handle unicode characters", func() {
			template := "Unicode: [[ .CHARS ]]"
			data := map[string]string{
				"CHARS": "Hello 世界 🌍",
			}

			result, err := RenderTemplate(template, data)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal("Unicode: Hello 世界 🌍"))
		})
	})

	Context("Complex templating scenarios", func() {
		It("should handle realistic Alertmanager configuration", func() {
			template := `global:
  smtp_smarthost: '[[ .SMTP_HOST ]]'
  smtp_from: '[[ .SMTP_FROM ]]'
  smtp_require_tls: [[ .SMTP_TLS | default "false" ]]

receivers:
  - name: 'team'
    email_configs:
      - to: '[[ .TEAM_EMAIL ]]'
        send_resolved: true`

			data := map[string]string{
				"SMTP_HOST":  "smtp.example.com:587",
				"SMTP_FROM":  "alertmanager@example.com",
				"SMTP_TLS":   "true",
				"TEAM_EMAIL": "team@example.com",
			}

			result, err := RenderTemplate(template, data)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring("smtp_smarthost: 'smtp.example.com:587'"))
			Expect(result).To(ContainSubstring("smtp_from: 'alertmanager@example.com'"))
			Expect(result).To(ContainSubstring("smtp_require_tls: true"))
			Expect(result).To(ContainSubstring("to: 'team@example.com'"))
		})

		It("should handle mixed variables and defaults", func() {
			template := `config:
  required: [[ .REQUIRED ]]
  optional: [[ .OPTIONAL | default "default-value" ]]
  another: [[ .ANOTHER | default "another-default" ]]`

			data := map[string]string{
				"REQUIRED": "present",
				"OPTIONAL": "also-present",
			}

			result, err := RenderTemplate(template, data)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring("required: present"))
			Expect(result).To(ContainSubstring("optional: also-present"))
			Expect(result).To(ContainSubstring("another: another-default"))
		})
	})
})
