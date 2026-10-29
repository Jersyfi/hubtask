// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package intake

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"context"
	"io"
	"strconv"
	"strings"

	cmdbservice "github.com/Jersyfi/hubtask/core/application/service/cmdb"
	"github.com/Jersyfi/hubtask/core/domain/model/cmdb"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// InventoryDeliverer is the application's side of an inventory intake.
type InventoryDeliverer interface {
	DeliverInventory(ctx context.Context, d cmdbservice.AlertDelivery, parse cmdbservice.InventoryParser) (cmdbservice.InventoryReceipt, error)
}

// InventoryIntake translates a GLPI Agent's native inventory into a payload of the
// reconciliation (research itsm-cmdb-ai §4.6): the computer it runs on and the software installed
// there. Which CI that is, and what may change, is decided inwards.
type InventoryIntake struct {
	Deliveries InventoryDeliverer
}

// Deliver hands the post and its credential on, with the parser of the intake's kind.
func (i InventoryIntake) Deliver(ctx context.Context, d cmdbservice.AlertDelivery) (cmdbservice.InventoryReceipt, error) {
	return i.Deliveries.DeliverInventory(ctx, d, ParseInventory)
}

// MaxInventoryBytes bounds a post once inflated: a compressed body is no measure of what it
// becomes, and the request bound is applied before inflating.
const MaxInventoryBytes = 16 << 20

func inventoryMalformed() error {
	return shared.ErrMalformedRequest.WithDetail("cmdb.inventory_malformed")
}

// ParseInventory reads a post in its kind's format.
func ParseInventory(kind cmdb.ConnectorKind, body []byte) (cmdbservice.InventoryDocument, error) {
	if kind != cmdb.ConnectorGLPIAgent {
		return cmdbservice.InventoryDocument{}, inventoryMalformed()
	}
	plain, err := inflate(body)
	if err != nil {
		return cmdbservice.InventoryDocument{}, err
	}
	return glpiAgent(plain)
}

// inflate undoes the compression an agent applies by default - zlib, or gzip - recognised by its
// first bytes, since an agent's content type is not always one a proxy keeps.
func inflate(body []byte) ([]byte, error) {
	var r io.Reader
	switch {
	case len(body) >= 2 && body[0] == 0x1f && body[1] == 0x8b:
		z, err := gzip.NewReader(bytes.NewReader(body))
		if err != nil {
			return nil, inventoryMalformed()
		}
		r = z
	case len(body) >= 2 && body[0] == 0x78:
		z, err := zlib.NewReader(bytes.NewReader(body))
		if err != nil {
			return nil, inventoryMalformed()
		}
		r = z
	default:
		return body, nil
	}
	out, err := io.ReadAll(io.LimitReader(r, MaxInventoryBytes+1))
	if err != nil || len(out) > MaxInventoryBytes {
		return nil, inventoryMalformed()
	}
	return out, nil
}

type glpiDocument struct {
	Action   string `json:"action"`
	DeviceID string `json:"deviceid"`
	ItemType string `json:"itemtype"`
	Content  struct {
		Hardware struct {
			Name        string `json:"name"`
			UUID        string `json:"uuid"`
			ChassisType string `json:"chassis_type"`
		} `json:"hardware"`
		Bios struct {
			SerialNumber string `json:"ssn"`
			Manufacturer string `json:"smanufacturer"`
			Model        string `json:"smodel"`
		} `json:"bios"`
		OperatingSystem struct {
			FullName  string `json:"full_name"`
			Name      string `json:"name"`
			Version   string `json:"version"`
			FQDN      string `json:"fqdn"`
			DNSDomain string `json:"dns_domain"`
		} `json:"operatingsystem"`
		Networks []struct {
			IPAddress  string `json:"ipaddress"`
			MACAddress string `json:"macaddr"`
			Virtual    any    `json:"virtualdev"`
		} `json:"networks"`
		Softwares []struct {
			Name      string `json:"name"`
			Version   string `json:"version"`
			Publisher string `json:"publisher"`
		} `json:"softwares"`
	} `json:"content"`
}

// computerRef names the agent's machine within its payload.
const computerRef = "computer"

// glpiAgent reads GLPI's native inventory format: a `contact` asks for tasks, an `inventory`
// describes one computer. The SMBIOS UUID is the native key - the agent's own device id changes
// when it is installed again - and its software is related to it as installed there.
func glpiAgent(body []byte) (cmdbservice.InventoryDocument, error) {
	var doc glpiDocument
	if err := decodeInto(body, &doc); err != nil {
		return cmdbservice.InventoryDocument{}, inventoryMalformed()
	}
	switch strings.ToLower(doc.Action) {
	case "contact":
		return cmdbservice.InventoryDocument{Contact: true}, nil
	case "inventory":
	default:
		return cmdbservice.InventoryDocument{}, inventoryMalformed()
	}
	if doc.ItemType != "" && !strings.EqualFold(doc.ItemType, "Computer") {
		return cmdbservice.InventoryDocument{}, shared.ErrValidation.WithDetail("cmdb.inventory_item_unsupported")
	}
	c := doc.Content
	name := strings.TrimSpace(c.Hardware.Name)
	native := strings.ToLower(strings.TrimSpace(c.Hardware.UUID))
	if native == "" {
		native = strings.TrimSpace(doc.DeviceID)
	}
	if name == "" || native == "" {
		return cmdbservice.InventoryDocument{}, inventoryMalformed()
	}
	system := strings.TrimSpace(c.OperatingSystem.FullName)
	if system == "" {
		system = strings.TrimSpace(c.OperatingSystem.Name + " " + c.OperatingSystem.Version)
	}
	domain := strings.TrimSpace(c.OperatingSystem.DNSDomain)
	if domain == "" {
		if _, rest, ok := strings.Cut(strings.TrimSpace(c.OperatingSystem.FQDN), "."); ok {
			domain = rest
		}
	}
	attributes := map[string]any{"hostname": strings.ToLower(name)}
	set := func(key, value string) {
		if value = strings.TrimSpace(value); value != "" {
			attributes[key] = value
		}
	}
	set("domain", strings.ToLower(domain))
	set("serial_number", c.Bios.SerialNumber)
	set("manufacturer", c.Bios.Manufacturer)
	set("model", c.Bios.Model)
	set("operating_system", system)
	for _, n := range c.Networks {
		if virtual(n.Virtual) || n.MACAddress == "" || n.IPAddress == "" || strings.HasPrefix(n.IPAddress, "127.") {
			continue
		}
		set("ip_address", n.IPAddress)
		set("mac_address", strings.ToLower(n.MACAddress))
		break
	}
	var p cmdb.Payload
	p.Items = append(p.Items, cmdb.Item{
		Ref: computerRef, Class: computerClass(c.Hardware.ChassisType, system), Name: name, NativeKey: native, Attributes: attributes,
	})
	seen := map[string]bool{}
	for _, s := range c.Softwares {
		label := strings.TrimSpace(s.Name)
		key := strings.ToLower(label) + "\x00" + strings.TrimSpace(s.Version)
		if label == "" || seen[key] || len(p.Items) >= cmdb.MaxPayloadItems || len(p.Relations) >= cmdb.MaxPayloadRelations {
			continue
		}
		seen[key] = true
		ref := "software-" + strconv.Itoa(len(p.Items))
		item := cmdb.Item{Ref: ref, Class: "software", Name: label, Attributes: map[string]any{}}
		if v := strings.TrimSpace(s.Version); v != "" {
			item.Attributes["version"] = v
		}
		if v := strings.TrimSpace(s.Publisher); v != "" {
			item.Attributes["publisher"] = v
		}
		p.Items = append(p.Items, item)
		p.Relations = append(p.Relations, cmdb.RelationItem{Type: cmdb.RelInstalled, From: ref, To: computerRef})
	}
	return cmdbservice.InventoryDocument{Payload: p}, nil
}

// computerClass is a laptop by its chassis, a server by its chassis or its system, otherwise a
// workstation.
func computerClass(chassis, system string) string {
	c, s := strings.ToLower(chassis), strings.ToLower(system)
	switch {
	case strings.Contains(c, "laptop") || strings.Contains(c, "notebook") || strings.Contains(c, "portable"):
		return "laptop"
	case strings.Contains(c, "server") || strings.Contains(c, "rack") || strings.Contains(c, "blade") || strings.Contains(s, "server"):
		return "server"
	}
	return "workstation"
}

func virtual(v any) bool {
	switch x := v.(type) {
	case bool:
		return x
	case string:
		return x == "1" || strings.EqualFold(x, "true")
	}
	return str(v) == "1"
}
