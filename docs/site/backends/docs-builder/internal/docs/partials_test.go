// Copyright 2026 Flant JSC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package docs

import (
	"strings"
	"testing"
)

func TestValidatePartialPath(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{name: "root partial", path: "/partials/setup-guide.md"},
		{name: "nested partial", path: "/partials/setup/basic-setup-v2.ru.md"},
		{name: "static asset", path: "/partials/static/images/diagram.png"},
		{name: "nested static directory", path: "/partials/setup/static/diagram.md", wantErr: true},
		{name: "uppercase file", path: "/partials/Setup.md", wantErr: true},
		{name: "underscore", path: "/partials/setup_guide.md", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validatePartialPath(tt.path)
			if (err != nil) != tt.wantErr {
				t.Fatalf("validatePartialPath() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestPreparePartialMarkdown(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		checkFn func(t *testing.T, got string)
	}{
		{
			name:  "inject front matter to plain markdown",
			input: "## Heading\n\nBody\n",
			checkFn: func(t *testing.T, got string) {
				required := []string{
					"layout: reusable-partial",
					"renderAsPartial: true",
					"searchable: false",
					"sitemap_include: false",
					"permalink: en/modules/test-module/alpha/partials/test-partial.html",
					"lang: en",
					"## Heading",
				}
				for _, item := range required {
					if !strings.Contains(got, item) {
						t.Fatalf("result does not contain %q: %s", item, got)
					}
				}
			},
		},
		{
			name: "preserve existing front matter",
			input: "---\n" +
				"title: Test\n" +
				"---\n\n" +
				"Body\n",
			checkFn: func(t *testing.T, got string) {
				if !strings.Contains(got, "title: Test") {
					t.Fatalf("existing front matter was lost: %s", got)
				}
				if !strings.Contains(got, "layout: reusable-partial") {
					t.Fatalf("layout was not injected: %s", got)
				}
			},
		},
		{
			name:  "derive ru permalink from file name",
			input: "## Заголовок\n",
			checkFn: func(t *testing.T, got string) {
				if !strings.Contains(got, "permalink: ru/modules/test-module/alpha/partials/test-partial.html") {
					t.Fatalf("unexpected ru permalink: %s", got)
				}
				if !strings.Contains(got, "lang: ru") {
					t.Fatalf("ru lang missing: %s", got)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fileName := "test-partial.md"
			if tt.name == "derive ru permalink from file name" {
				fileName = "test-partial.ru.md"
			}
			got := string(preparePartialMarkdown([]byte(tt.input), "test-module", "alpha", fileName))
			tt.checkFn(t, got)
		})
	}
}
