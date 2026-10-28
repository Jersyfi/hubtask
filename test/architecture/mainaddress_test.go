// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package architecture

import (
	"go/ast"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

// addressHelper is the one file that turns the installation's address into a workspace's
// (ADR-0095 §2, UC-INS-19 check 9).
const addressHelper = "core/application/service/identity/WorkspaceAddress.go"

// Every mail and link to a workspace is built on its main row. A writer that read the
// installation's BaseURL, or composed the address from the short name, would keep the host a change
// of address left behind - and a mail that links there reaches a page that knows neither the
// workspace nor the session. So outside the one helper, nothing in the application layer reads a
// BaseURL or calls WorkspaceBase/WorkspaceBaseAt.
func TestNoLinkWriterReadsTheInstallationsAddress(t *testing.T) {
	forEachGoFile(t, []string{"../../core/application"},
		func(path string, file *ast.File, fset *token.FileSet) {
			if strings.HasSuffix(path, "_test.go") || filepath.ToSlash(rel(path)) == addressHelper {
				return
			}
			ast.Inspect(file, func(node ast.Node) bool {
				selector, ok := node.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				switch selector.Sel.Name {
				case "WorkspaceBase", "WorkspaceBaseAt":
					t.Errorf("%s:%d: composes a workspace's address itself - build the link on "+
						"identity.WorkspaceAddress, the main row's (ADR-0095 §2)",
						rel(path), fset.Position(selector.Pos()).Line)
				case "BaseURL":
					if linkWriter(path) {
						t.Errorf("%s:%d: a mail or link writer reads BaseURL - build the link on "+
							"identity.WorkspaceAddress, the main row's (ADR-0095 §2)",
							rel(path), fset.Position(selector.Pos()).Line)
					}
				}
				return true
			})
		})
}

// linkWriter is the code that writes links to a workspace: the mails, and the identity service's
// invitations and returns. An AI provider's or a webhook's BaseURL elsewhere is another address.
func linkWriter(path string) bool {
	p := filepath.ToSlash(rel(path))
	return strings.HasPrefix(p, "core/application/service/notification/") ||
		strings.HasPrefix(p, "core/application/service/identity/")
}
