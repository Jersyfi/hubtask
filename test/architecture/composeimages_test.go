// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package architecture

import (
	"regexp"
	"strings"
	"testing"
)

// The database image of the Compose stacks is pinned to a release and its digest. A floating tag
// (`pg16`) is re-pushed on every PostgreSQL minor and pgvector release, so the same file pulled a
// week apart runs a different server and an upgrade is not reproducible (deployment.md §2.1).
// Dependabot moves the pin (.github/dependabot.yml). The reference stack's db, upgrade-dump and ops
// share one image, so pg_dump and pg_restore always match the server.
func TestTheComposeDatabaseImageIsPinnedByDigest(t *testing.T) {
	pinned := regexp.MustCompile(`^pgvector/pgvector:\d+\.\d+\.\d+-pg\d+@sha256:[0-9a-f]{64}$`)
	image := regexp.MustCompile(`(?m)^\s+image:\s*(\S+)\s*$`)
	var seen []string
	for _, file := range []string{"../../deploy/docker/compose.yaml", "../../deploy/docker/compose.dev.yaml"} {
		for _, m := range image.FindAllStringSubmatch(string(readFile(t, file)), -1) {
			if strings.HasPrefix(m[1], "${HUBTASK_IMAGE") {
				continue
			}
			if !pinned.MatchString(m[1]) {
				t.Errorf("%s: image %q is not a pgvector release pinned by digest", file, m[1])
			}
			seen = append(seen, m[1])
		}
	}
	if len(seen) != 4 {
		t.Errorf("found %d database images, want the reference stack's three and the dev stack's one", len(seen))
	}
	for _, s := range seen {
		if s != seen[0] {
			t.Errorf("the stacks run %q beside %q; pg_dump and pg_restore must match the server", s, seen[0])
		}
	}
}
