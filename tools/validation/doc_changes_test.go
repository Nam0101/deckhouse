package main

import (
	"strings"
	"testing"
)

func TestValidateModuleIncludePlaceholders(t *testing.T) {
	t.Run("valid placeholder", func(t *testing.T) {
		content := strings.Join([]string{
			`Some text.`,
			`<script type="application/x-module-include">`,
			`{"module":"operator-trivy","channel":"alpha","artifact":"feature-test1.md","onError":"fallback","fallback":"<a href=\"/modules/operator-trivy/alpha/\">Open module docs</a>"}`,
			`</script>`,
		}, "\n")

		errors := validateModuleIncludePlaceholders(content)
		if len(errors) != 0 {
			t.Fatalf("expected no errors, got %v", errors)
		}
	})

	t.Run("reject language suffix in artifact", func(t *testing.T) {
		content := `<script type="application/x-module-include">{"module":"operator-trivy","artifact":"feature-test1.ru.md"}</script>`

		errors := validateModuleIncludePlaceholders(content)
		if len(errors) != 1 || !strings.Contains(errors[0], "must not contain language suffix") {
			t.Fatalf("expected language suffix validation error, got %v", errors)
		}
	})

	t.Run("require fallback body", func(t *testing.T) {
		content := `<script type="application/x-module-include">{"module":"operator-trivy","artifact":"feature-test1.md","onError":"fallback"}</script>`

		errors := validateModuleIncludePlaceholders(content)
		if len(errors) != 1 || !strings.Contains(errors[0], "fallback content is required") {
			t.Fatalf("expected fallback validation error, got %v", errors)
		}
	})

	t.Run("reject invalid json", func(t *testing.T) {
		content := `<script type="application/x-module-include">{"module":"operator-trivy",}</script>`

		errors := validateModuleIncludePlaceholders(content)
		if len(errors) != 1 || !strings.Contains(errors[0], "invalid JSON") {
			t.Fatalf("expected invalid json error, got %v", errors)
		}
	})
}
