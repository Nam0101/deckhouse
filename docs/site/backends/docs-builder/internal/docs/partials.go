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
	"bytes"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

var partialPathSegmentRegexp = regexp.MustCompile(`^[a-z0-9-]+$`)
var partialMarkdownFileRegexp = regexp.MustCompile(`^[a-z0-9-]+(?:-v[0-9]+)?(?:\.ru)?\.md$`)

func isPartialMarkdownPath(path string) bool {
	return strings.HasPrefix(path, "/partials/") &&
		!strings.HasPrefix(path, "/partials/static/") &&
		(strings.HasSuffix(path, ".md") || strings.HasSuffix(path, ".ru.md"))
}

func isPartialStaticPath(path string) bool {
	return strings.HasPrefix(path, "/partials/static/")
}

func validatePartialPath(path string) error {
	if !strings.HasPrefix(path, "/partials/") {
		return nil
	}

	if strings.Contains(strings.TrimPrefix(path, "/partials/"), "/static/") && !strings.HasPrefix(path, "/partials/static/") {
		return fmt.Errorf("nested static directory is not allowed in partials path: %s", path)
	}

	if isPartialStaticPath(path) {
		return nil
	}

	if !isPartialMarkdownPath(path) {
		return fmt.Errorf("unsupported partial artifact path: %s", path)
	}

	segments := strings.Split(strings.TrimPrefix(path, "/partials/"), "/")
	for i, segment := range segments {
		if i == len(segments)-1 {
			if !partialMarkdownFileRegexp.MatchString(segment) {
				return fmt.Errorf("invalid partial file name: %s", segment)
			}
			continue
		}

		if segment == "static" || !partialPathSegmentRegexp.MatchString(segment) {
			return fmt.Errorf("invalid partial path segment: %s", segment)
		}
	}

	return nil
}

func preparePartialMarkdown(raw []byte, moduleName, channel, fileName string) []byte {
	lang := "en"
	permalinkName := strings.TrimSuffix(fileName, ".md")
	if strings.HasSuffix(fileName, ".ru.md") {
		lang = "ru"
		permalinkName = strings.TrimSuffix(fileName, ".ru.md")
	}

	permalink := fmt.Sprintf("%s/modules/%s/%s/partials/%s.html", lang, moduleName, channel, permalinkName)
	generatedFrontMatter := fmt.Sprintf(
		"layout: reusable-partial\nrenderAsPartial: true\nsearchable: false\nsitemap_include: false\npermalink: %s\nlang: %s\n",
		permalink,
		lang,
	)

	if !hasYAMLFrontMatter(raw) {
		result := bytes.NewBuffer(make([]byte, 0, len(raw)+len(generatedFrontMatter)+16))
		result.WriteString("---\n")
		result.WriteString(generatedFrontMatter)
		result.WriteString("---\n\n")
		result.Write(raw)
		return result.Bytes()
	}

	lines := bytes.Split(raw, []byte("\n"))
	if len(lines) < 2 {
		return raw
	}

	header := bytes.Join(lines[:findFrontMatterEnd(lines)], []byte("\n"))
	headerStr := string(header)

	injected := make([]string, 0, 4)
	if !strings.Contains(headerStr, "\nlayout:") && !strings.HasPrefix(headerStr, "layout:") {
		injected = append(injected, "layout: reusable-partial")
	}
	if !strings.Contains(headerStr, "\nrenderAsPartial:") && !strings.HasPrefix(headerStr, "renderAsPartial:") {
		injected = append(injected, "renderAsPartial: true")
	}
	if !strings.Contains(headerStr, "\nsearchable:") && !strings.HasPrefix(headerStr, "searchable:") {
		injected = append(injected, "searchable: false")
	}
	if !strings.Contains(headerStr, "\nsitemap_include:") && !strings.HasPrefix(headerStr, "sitemap_include:") {
		injected = append(injected, "sitemap_include: false")
	}
	if len(injected) == 0 {
		return raw
	}

	result := bytes.NewBuffer(make([]byte, 0, len(raw)+128))
	result.Write(lines[0])
	result.WriteByte('\n')
	for _, line := range injected {
		result.WriteString(line)
		result.WriteByte('\n')
	}
	for i := 1; i < len(lines); i++ {
		result.Write(lines[i])
		if i != len(lines)-1 {
			result.WriteByte('\n')
		}
	}

	return result.Bytes()
}

func hasYAMLFrontMatter(raw []byte) bool {
	lines := bytes.Split(raw, []byte("\n"))
	if len(lines) < 3 || string(lines[0]) != "---" {
		return false
	}

	for i := 1; i < len(lines); i++ {
		if string(lines[i]) == "---" {
			return true
		}
	}

	return false
}

func findFrontMatterEnd(lines [][]byte) int {
	if len(lines) == 0 || string(lines[0]) != "---" {
		return 0
	}

	for i := 1; i < len(lines); i++ {
		if string(lines[i]) == "---" {
			return i
		}
	}

	return len(lines)
}

func partialStaticOutputPath(baseDir, moduleName, channel string) string {
	return filepath.Join(baseDir, partialsDir, moduleName, channel, "partials", "static")
}
