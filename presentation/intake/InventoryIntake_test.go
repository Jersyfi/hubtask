// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package intake

import (
	"bytes"
	"compress/zlib"
	"errors"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/cmdb"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

const agentInventory = `{"action":"inventory","deviceid":"pc-42-2026-10-30-10-00-00","itemtype":"Computer","content":{
  "hardware":{"name":"PC-42","uuid":"4C4C4544-0042-3010-8058-B3C04F4B4D32","chassis_type":"Notebook"},
  "bios":{"ssn":"5KMZ2X3","smanufacturer":"Dell Inc.","smodel":"Latitude 7440"},
  "operatingsystem":{"full_name":"Debian GNU/Linux 12 (bookworm)","fqdn":"pc-42.office.example"},
  "networks":[{"description":"lo","ipaddress":"127.0.0.1","macaddr":"00:00:00:00:00:00"},
    {"description":"docker0","ipaddress":"172.17.0.1","macaddr":"02:42:AC:11:00:01","virtualdev":1},
    {"description":"wlan0","ipaddress":"10.0.4.42","macaddr":"A4:C3:F0:11:22:33","virtualdev":0}],
  "softwares":[{"name":"firefox-esr","version":"128.3.1","publisher":"Mozilla"},{"name":"firefox-esr","version":"128.3.1"},
    {"name":"openssl","version":"3.0.14"},{"name":""}]}}`

func TestAGLPIAgentsInventoryBecomesItsComputerWithItsSoftware(t *testing.T) {
	var zipped bytes.Buffer
	w := zlib.NewWriter(&zipped)
	_, _ = w.Write([]byte(agentInventory))
	_ = w.Close()
	for name, body := range map[string][]byte{"plain": []byte(agentInventory), "zlib": zipped.Bytes()} {
		t.Run(name, func(t *testing.T) {
			doc, err := ParseInventory(cmdb.ConnectorGLPIAgent, body)
			if err != nil || doc.Contact {
				t.Fatalf("%+v %v", doc, err)
			}
			items := doc.Payload.Items
			if len(items) != 3 || len(doc.Payload.Relations) != 2 {
				t.Fatalf("one computer and its two distinct programs: %+v", doc.Payload)
			}
			pc := items[0]
			want := map[string]any{
				"hostname": "pc-42", "domain": "office.example", "serial_number": "5KMZ2X3", "manufacturer": "Dell Inc.",
				"model": "Latitude 7440", "operating_system": "Debian GNU/Linux 12 (bookworm)", "ip_address": "10.0.4.42",
				"mac_address": "a4:c3:f0:11:22:33",
			}
			if pc.Class != "laptop" || pc.Name != "PC-42" || pc.NativeKey != "4c4c4544-0042-3010-8058-b3c04f4b4d32" {
				t.Fatalf("the machine by its SMBIOS UUID: %+v", pc)
			}
			for k, v := range want {
				if pc.Attributes[k] != v {
					t.Fatalf("%s: %v, want %v (the loopback and the virtual interface are skipped)", k, pc.Attributes[k], v)
				}
			}
			if items[1].Class != "software" || items[1].Attributes["version"] != "128.3.1" || items[1].Attributes["publisher"] != "Mozilla" {
				t.Fatalf("software: %+v", items[1])
			}
			r := doc.Payload.Relations[0]
			if r.Type != cmdb.RelInstalled || r.From != items[1].Ref || r.To != pc.Ref {
				t.Fatalf("installed on the computer: %+v", r)
			}
		})
	}
}

func TestAGLPIAgentsContactIsAnsweredAndAnythingElseRefused(t *testing.T) {
	doc, err := ParseInventory(cmdb.ConnectorGLPIAgent, []byte(`{"action":"contact","deviceid":"pc-42","name":"GLPI-Agent_v1.11"}`))
	if err != nil || !doc.Contact {
		t.Fatalf("a contact: %+v %v", doc, err)
	}
	cases := map[string]struct {
		kind cmdb.ConnectorKind
		body string
		code string
	}{
		"not JSON":           {cmdb.ConnectorGLPIAgent, `<?xml version="1.0"?><REQUEST/>`, "cmdb.inventory_malformed"},
		"an unknown action":  {cmdb.ConnectorGLPIAgent, `{"action":"netdiscovery"}`, "cmdb.inventory_malformed"},
		"no name":            {cmdb.ConnectorGLPIAgent, `{"action":"inventory","deviceid":"d","content":{}}`, "cmdb.inventory_malformed"},
		"a network device":   {cmdb.ConnectorGLPIAgent, `{"action":"inventory","itemtype":"NetworkEquipment","content":{}}`, "cmdb.inventory_item_unsupported"},
		"another kind":       {cmdb.ConnectorAlertmanager, agentInventory, "cmdb.inventory_malformed"},
		"a broken zlib body": {cmdb.ConnectorGLPIAgent, "\x78\x9cnot zlib", "cmdb.inventory_malformed"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := ParseInventory(c.kind, []byte(c.body))
			if e := (*shared.Error)(nil); !errors.As(err, &e) || e.DetailCode != c.code {
				t.Fatalf("got %v, want %s", err, c.code)
			}
		})
	}
}
