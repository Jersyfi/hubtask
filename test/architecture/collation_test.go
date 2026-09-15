// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package architecture

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Every read that orders by a name a person wrote says which collation it orders under (M-08,
// i18n-l10n.md §5).
//
// `ORDER BY name` sorts in whatever collation the database was created with, which differs by
// installation; `hubtask_name` is the one migration 0080 defines so that "Ä" sits beside "A"
// everywhere. A query that forgets it is not wrong on the development stack - en_US.utf8 agrees
// with ICU on the cases anybody notices - and wrong on the next container, which is why this is a
// gate rather than a review note. A rank key is the other case and says `COLLATE "C"` for the
// opposite reason (migration 0007); this gate leaves it alone.
func TestEveryNameOrderingNamesItsCollation(t *testing.T) {
	// A text column a person reads and would expect in alphabetical order.
	nameColumn := regexp.MustCompile(`\b(?:[a-z]+\.)?(?:name|title|display_name|email)\)?(?:\s*,|\s*;|\s*$|\s+(?:ASC|DESC))`)

	files, err := filepath.Glob("../../db/queries/*.sql")
	if err != nil || len(files) == 0 {
		t.Fatalf("no queries found: %v", err)
	}
	for _, path := range files {
		file, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		scanner := bufio.NewScanner(file)
		line := 0
		for scanner.Scan() {
			line++
			text := strings.TrimSpace(scanner.Text())
			if strings.HasPrefix(text, "--") || !strings.Contains(text, "ORDER BY") {
				continue
			}
			clause := text[strings.Index(text, "ORDER BY")+len("ORDER BY"):]
			for _, term := range strings.Split(clause, ",") {
				term = strings.TrimSpace(term)
				if !nameColumn.MatchString(term+";") || strings.Contains(term, "COLLATE") {
					continue
				}
				t.Errorf("%s:%d orders by a name without naming its collation: %q - say "+
					"COLLATE hubtask_name (migration 0080, i18n-l10n.md §5)", rel(path), line, term)
			}
		}
		_ = file.Close()
	}
}
