// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package locales

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Hubtask's templates carry their key as the name and their questions as keys, and a client names
// both in the reader's language (rule 8): every shipped template and every question it asks has its
// words in each catalogue.
func TestEveryShippedTemplateAndQuestionHasItsWords(t *testing.T) {
	files, err := filepath.Glob("../core/application/service/config/shipped/*.json")
	if err != nil || len(files) == 0 {
		t.Fatalf("no shipped template found: %v", err)
	}
	catalogues := map[string]map[string]string{"en.json": catalogue(t, "en.json"), "de.json": catalogue(t, "de.json")}
	for _, file := range files {
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		var template struct {
			Key       string `json:"key"`
			Questions []struct {
				Key string `json:"key"`
			} `json:"questions"`
		}
		if err := json.Unmarshal(raw, &template); err != nil {
			t.Fatalf("%s: %v", file, err)
		}
		codes := []string{"app.structure.preset." + strings.ToUpper(template.Key)}
		for _, question := range template.Questions {
			codes = append(codes, "app.templates.question."+question.Key)
		}
		for name, messages := range catalogues {
			for _, code := range codes {
				if messages[code] == "" {
					t.Errorf("%s has no %s (%s)", name, code, filepath.Base(file))
				}
			}
		}
	}
}
