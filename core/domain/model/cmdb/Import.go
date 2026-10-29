// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package cmdb

import (
	"strings"
)

// MaxFileBytes bounds an imported file.
const MaxFileBytes = 5 << 20

// FileFormat is the shape of an imported file (research itsm-cmdb-ai C4).
type FileFormat string

const (
	// FormatCSV is a table with a header row.
	FormatCSV FileFormat = "CSV"
	// FormatJSON is an array of objects, one per CI.
	FormatJSON FileFormat = "JSON"
	// FormatCycloneDX and FormatSPDX are software bills of materials: each component or package
	// becomes a `software` CI identified by its package URL, CPE or name and version.
	FormatCycloneDX FileFormat = "CYCLONEDX"
	FormatSPDX      FileFormat = "SPDX"
)

// FileMapping says how a table's columns become a CI (CSV and JSON): the class, which column is
// the name and which the source's own key, which columns are attributes, and defaults for what
// the file leaves out.
type FileMapping struct {
	Class string
	// ClassColumn, when set, reads each row's class from the column; Class is the fallback.
	ClassColumn     string
	NameColumn      string
	NativeKeyColumn string
	// Columns maps a column to an attribute key.
	Columns  map[string]string
	Defaults map[string]string
}

// Check refuses a mapping to an abstract or unknown class or without a name column.
func (m FileMapping) Check(classes Classes) error {
	if c, ok := classes.Find(m.Class); !ok || c.Abstract {
		return invalid("/mapping/class", "cmdb.class_not_instantiable", "key", m.Class)
	}
	if strings.TrimSpace(m.NameColumn) == "" {
		return invalid("/mapping/name_column", "cmdb.mapping_name_column_required")
	}
	if len(m.Columns) > MaxAttributes {
		return invalid("/mapping/columns", "cmdb.class_too_many_attributes")
	}
	return nil
}
