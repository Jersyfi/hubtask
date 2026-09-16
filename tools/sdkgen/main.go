// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Command sdkgen generates the TypeScript and the Python client from api/openapi.yaml (P-03).
//
//	sdkgen <openapi.yaml> <client.gen.ts> <python package dir>
//
// Two targets, one reading of the document, no dependency in either output. The Go client is
// oapi-codegen's (sdk/go); these two are the ones no tool in the toolchain generates, and a
// generator of a fifth of OpenAPI is smaller than the dependency that would generate them.
package main

import (
	"fmt"
	"os"
	"path/filepath"
)

func main() {
	if len(os.Args) != 4 {
		fmt.Fprintln(os.Stderr, "usage: sdkgen <openapi.yaml> <client.gen.ts> <python package dir>")
		os.Exit(2)
	}
	raw, err := os.ReadFile(os.Args[1]) //nolint:gosec // G703: the file the Makefile names; reading it is the job
	if err != nil {
		fail(err)
	}
	doc, err := Read(raw)
	if err != nil {
		fail(err)
	}
	if err := write(os.Args[2], GenerateTypeScript(doc)); err != nil {
		fail(err)
	}
	client, types := GeneratePython(doc)
	if err := write(filepath.Join(os.Args[3], "client.py"), client); err != nil {
		fail(err)
	}
	if err := write(filepath.Join(os.Args[3], "types.py"), types); err != nil {
		fail(err)
	}
	fmt.Printf("sdkgen: %d operations, %d schemas -> %s, %s\n", len(doc.Operations), len(doc.Schemas), os.Args[2], os.Args[3])
}

func write(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil { //nolint:gosec // G703: the output the Makefile names
		return err
	}
	// 0o644: generated source of the repository, read by everything and written by this.
	return os.WriteFile(path, []byte(content), 0o644) //nolint:gosec // G306, G703: the outputs the Makefile names
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
