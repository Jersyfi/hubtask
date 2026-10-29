// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package cmdb

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"io"
	"strconv"
	"strings"

	domain "github.com/Jersyfi/hubtask/core/domain/model/cmdb"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

func fileInvalid(path, code string, params ...string) error {
	err := shared.ErrValidation.WithDetail(code).WithFields(shared.FieldError{Path: path, Code: code})
	if len(params) == 2 {
		err = err.WithParams(map[string]string{params[0]: params[1]})
	}
	return err
}

// ParseFile reads a file into a payload. A row that cannot be read is reported by its number and
// left out; the rest go on to the reconciliation, which decides each item.
func ParseFile(format domain.FileFormat, data []byte, mapping *domain.FileMapping) (domain.Payload, []int, error) {
	if len(data) > domain.MaxFileBytes {
		return domain.Payload{}, nil, fileInvalid("/content", "cmdb.file_too_large", "maximum", strconv.Itoa(domain.MaxFileBytes))
	}
	switch format {
	case domain.FormatCSV, domain.FormatJSON:
		if mapping == nil {
			return domain.Payload{}, nil, fileInvalid("/mapping", "cmdb.mapping_required")
		}
		rows, err := tableRows(format, data)
		if err != nil {
			return domain.Payload{}, nil, err
		}
		return mapRows(rows, *mapping)
	case domain.FormatCycloneDX:
		return cycloneDX(data)
	case domain.FormatSPDX:
		return spdx(data)
	}
	return domain.Payload{}, nil, fileInvalid("/format", "cmdb.file_format_unknown")
}

func tableRows(format domain.FileFormat, data []byte) ([]map[string]string, error) {
	if format == domain.FormatJSON {
		var raw []map[string]any
		if err := json.Unmarshal(data, &raw); err != nil {
			return nil, fileInvalid("/content", "cmdb.file_unreadable")
		}
		rows := make([]map[string]string, 0, len(raw))
		for _, r := range raw {
			row := map[string]string{}
			for k, v := range r {
				switch t := v.(type) {
				case string:
					row[k] = t
				case float64:
					row[k] = strconv.FormatFloat(t, 'f', -1, 64)
				case bool:
					row[k] = strconv.FormatBool(t)
				}
			}
			rows = append(rows, row)
		}
		return rows, nil
	}
	reader := csv.NewReader(bytes.NewReader(bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))))
	reader.FieldsPerRecord = -1
	header, err := reader.Read()
	if err != nil {
		return nil, fileInvalid("/content", "cmdb.file_unreadable")
	}
	var rows []map[string]string
	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fileInvalid("/content", "cmdb.file_unreadable")
		}
		row := map[string]string{}
		for i, h := range header {
			if i < len(record) {
				row[strings.TrimSpace(h)] = record[i]
			}
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func mapRows(rows []map[string]string, m domain.FileMapping) (domain.Payload, []int, error) {
	if len(rows) > domain.MaxPayloadItems {
		return domain.Payload{}, nil, fileInvalid("/content", "cmdb.payload_too_large", "maximum", strconv.Itoa(domain.MaxPayloadItems))
	}
	var p domain.Payload
	var skipped []int
	for i, row := range rows {
		name := strings.TrimSpace(row[m.NameColumn])
		if name == "" {
			skipped = append(skipped, i+1)
			continue
		}
		class := m.Class
		if m.ClassColumn != "" && strings.TrimSpace(row[m.ClassColumn]) != "" {
			class = strings.TrimSpace(row[m.ClassColumn])
		}
		attrs := map[string]any{}
		for k, v := range m.Defaults {
			attrs[k] = v
		}
		for column, key := range m.Columns {
			if v, ok := row[column]; ok && strings.TrimSpace(v) != "" {
				attrs[key] = v
			}
		}
		p.Items = append(p.Items, domain.Item{
			Ref: "row-" + strconv.Itoa(i+1), Class: class, Name: name,
			NativeKey: strings.TrimSpace(row[m.NativeKeyColumn]), Attributes: attrs,
		})
	}
	return p, skipped, nil
}

func software(ref, name, version, publisher, purl, cpe string) domain.Item {
	attrs := map[string]any{}
	for k, v := range map[string]string{"version": version, "publisher": publisher, "purl": purl, "cpe": cpe} {
		if strings.TrimSpace(v) != "" {
			attrs[k] = v
		}
	}
	native := purl
	return domain.Item{Ref: ref, Class: "software", Name: name, NativeKey: native, Attributes: attrs}
}

func cycloneDX(data []byte) (domain.Payload, []int, error) {
	var doc struct {
		BomFormat  string `json:"bomFormat"`
		Components []struct {
			BomRef    string `json:"bom-ref"`
			Name      string `json:"name"`
			Version   string `json:"version"`
			Publisher string `json:"publisher"`
			Group     string `json:"group"`
			Purl      string `json:"purl"`
			Cpe       string `json:"cpe"`
		} `json:"components"`
		Dependencies []struct {
			Ref       string   `json:"ref"`
			DependsOn []string `json:"dependsOn"`
		} `json:"dependencies"`
	}
	if err := json.Unmarshal(data, &doc); err != nil || doc.BomFormat != "CycloneDX" {
		return domain.Payload{}, nil, fileInvalid("/content", "cmdb.file_unreadable")
	}
	var p domain.Payload
	var skipped []int
	refs := map[string]bool{}
	for i, c := range doc.Components {
		if strings.TrimSpace(c.Name) == "" || i >= domain.MaxPayloadItems {
			skipped = append(skipped, i+1)
			continue
		}
		ref := c.BomRef
		if ref == "" {
			ref = "component-" + strconv.Itoa(i+1)
		}
		publisher := c.Publisher
		if publisher == "" {
			publisher = c.Group
		}
		refs[ref] = true
		p.Items = append(p.Items, software(ref, c.Name, c.Version, publisher, c.Purl, c.Cpe))
	}
	for _, d := range doc.Dependencies {
		for _, on := range d.DependsOn {
			if refs[d.Ref] && refs[on] && len(p.Relations) < domain.MaxPayloadRelations {
				p.Relations = append(p.Relations, domain.RelationItem{Type: domain.RelDependsOn, From: d.Ref, To: on})
			}
		}
	}
	return p, skipped, nil
}

func spdx(data []byte) (domain.Payload, []int, error) {
	var doc struct {
		SPDXVersion string `json:"spdxVersion"`
		Packages    []struct {
			SPDXID       string `json:"SPDXID"`
			Name         string `json:"name"`
			VersionInfo  string `json:"versionInfo"`
			Supplier     string `json:"supplier"`
			ExternalRefs []struct {
				Type    string `json:"referenceType"`
				Locator string `json:"referenceLocator"`
			} `json:"externalRefs"`
		} `json:"packages"`
		Relationships []struct {
			Element string `json:"spdxElementId"`
			Type    string `json:"relationshipType"`
			Related string `json:"relatedSpdxElement"`
		} `json:"relationships"`
	}
	if err := json.Unmarshal(data, &doc); err != nil || !strings.HasPrefix(doc.SPDXVersion, "SPDX-") {
		return domain.Payload{}, nil, fileInvalid("/content", "cmdb.file_unreadable")
	}
	var p domain.Payload
	var skipped []int
	refs := map[string]bool{}
	for i, pkg := range doc.Packages {
		if strings.TrimSpace(pkg.Name) == "" || i >= domain.MaxPayloadItems {
			skipped = append(skipped, i+1)
			continue
		}
		var purl, cpe string
		for _, r := range pkg.ExternalRefs {
			switch {
			case r.Type == "purl":
				purl = r.Locator
			case strings.HasPrefix(r.Type, "cpe"):
				cpe = r.Locator
			}
		}
		supplier := strings.TrimPrefix(strings.TrimPrefix(pkg.Supplier, "Organization: "), "Person: ")
		if supplier == "NOASSERTION" {
			supplier = ""
		}
		refs[pkg.SPDXID] = true
		p.Items = append(p.Items, software(pkg.SPDXID, pkg.Name, pkg.VersionInfo, supplier, purl, cpe))
	}
	for _, r := range doc.Relationships {
		if r.Type == "DEPENDS_ON" && refs[r.Element] && refs[r.Related] && len(p.Relations) < domain.MaxPayloadRelations {
			p.Relations = append(p.Relations, domain.RelationItem{Type: domain.RelDependsOn, From: r.Element, To: r.Related})
		}
	}
	return p, skipped, nil
}
