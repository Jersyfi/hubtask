// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package odata

import (
	"encoding/xml"
	"fmt"
	"strings"
	"testing"
	"time"
)

func entries(n int) []Entry {
	at := time.Date(2026, 10, 30, 9, 0, 0, 0, time.UTC)
	out := make([]Entry, n)
	for i := range out {
		out[i] = Entry{ID: fmt.Sprintf("0192f000-0000-7000-8000-%012d", i), Number: int64(i + 1),
			Level: "TASK", Title: "Row", CollectionID: "0192f000-0000-7000-8000-00000000c001", CreatedAt: at, UpdatedAt: at}
	}
	return out
}

// The metadata is a well-formed CSDL document naming the set and its key (UC-INT-15 check 2).
func TestTheMetadataDescribesTheEntrySet(t *testing.T) {
	doc := Metadata()
	if err := xml.Unmarshal(doc, new(struct{})); err != nil {
		t.Fatalf("the metadata is not XML: %v", err)
	}
	for _, want := range []string{`Version="4.0"`, `<EntitySet Name="Entries" EntityType="Hubtask.Entry"/>`,
		`<PropertyRef Name="Id"/>`, `<Property Name="DueAt" Type="Edm.DateTimeOffset" Nullable="true"/>`} {
		if !strings.Contains(string(doc), want) {
			t.Errorf("the metadata lacks %s", want)
		}
	}
	if strings.Contains(string(doc), "Notes") {
		t.Error("the metadata carries notes")
	}
	if svc := Service("https://x/odata/t"); svc["@odata.context"] != "https://x/odata/t/$metadata" {
		t.Errorf("service %v", svc)
	}
}

// Rows are paged by the server at a thousand, and `$top` and `$skip` window them (UC-INT-15 check 3).
func TestEntriesArePagedAndWindowed(t *testing.T) {
	root := "https://x/odata/t"
	first := Entries(root, entries(2500), Page{})
	if n := len(first["value"].([]map[string]any)); n != PageSize {
		t.Errorf("first page %d rows", n)
	}
	if first["@odata.nextLink"] != root+"/Entries?$skip=1000" {
		t.Errorf("next link %v", first["@odata.nextLink"])
	}
	last := Entries(root, entries(2500), Page{Skip: 2000})
	if n := len(last["value"].([]map[string]any)); n != 500 || last["@odata.nextLink"] != nil {
		t.Errorf("last page %d rows, next %v", n, last["@odata.nextLink"])
	}
	top := Entries(root, entries(2500), Page{Skip: 10, Top: 5, HasTop: true})
	rows := top["value"].([]map[string]any)
	if len(rows) != 5 || rows[0]["Number"] != int64(11) || top["@odata.nextLink"] != nil {
		t.Errorf("windowed %v", top)
	}
	beyond := Entries(root, entries(3), Page{Skip: 10})
	if n := len(beyond["value"].([]map[string]any)); n != 0 {
		t.Errorf("beyond the end %d rows", n)
	}
	if rows[0]["DueAt"] != nil || rows[0]["AssigneeId"] != nil {
		t.Errorf("an empty value is not null: %v", rows[0])
	}
}
