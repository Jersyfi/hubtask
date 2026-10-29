// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package connector

import (
	"context"
	"encoding/json"
	"net/url"
	"strconv"
	"strings"

	"github.com/Jersyfi/hubtask/core/domain/model/cmdb"
	port "github.com/Jersyfi/hubtask/core/port/connector"
)

// glpi pulls GLPI's computers through its high-level API (v2) with an OAuth 2 client: changed
// since the newest modification the last walk saw, page by page. A computer whose type names a
// server is a server, any other a workstation; GLPI's own identifier is the native key.
func glpi(ctx context.Context, s session) (port.Page, error) {
	token, err := s.token(ctx, s.base("/api.php/token"), map[string]string{
		"client_id": s.setting("client_id"), "client_secret": s.secret(), "scope": "api",
	})
	if err != nil {
		return port.Page{}, err
	}
	c := readOffset(s.pull.Cursor)
	q := url.Values{"start": {strconv.Itoa(c.offset)}, "limit": {strconv.Itoa(s.size)}, "sort": {"date_mod:asc"}}
	if c.since != "" {
		q.Set("filter", "date_mod=ge="+c.since)
	}
	var rows []map[string]any
	if _, err := s.getJSON(ctx, s.base("/api.php/v2/Assets/Computer?"+q.Encode()), bearer(token), &rows); err != nil {
		return port.Page{}, err
	}
	var page port.Page
	for _, r := range rows {
		id, name := text(r["id"]), text(r["name"])
		if id == "" || name == "" {
			continue
		}
		class := "workstation"
		if strings.Contains(strings.ToLower(text(r["type"])), "server") {
			class = "server"
		}
		c.saw(text(r["date_mod"]))
		page.Payload.Items = append(page.Payload.Items, cmdb.Item{
			Ref: "glpi-" + id, Class: class, Name: name, NativeKey: id,
			Attributes: attrs("hostname", name, "serial_number", text(r["serial"]), "asset_tag", text(r["otherserial"]),
				"manufacturer", text(r["manufacturer"]), "model", text(r["model"])),
		})
	}
	page.Cursor, page.More = c.next(len(rows), s.size)
	return page, nil
}

// netbox pulls NetBox's devices with a token, changed since the newest the last walk saw. A
// device whose role names a server is a server, any other a network device; its primary
// address, without the prefix length, is its address.
func netbox(ctx context.Context, s session) (port.Page, error) {
	c := readOffset(s.pull.Cursor)
	q := url.Values{"limit": {strconv.Itoa(s.size)}, "offset": {strconv.Itoa(c.offset)}, "ordering": {"last_updated"}}
	if c.since != "" {
		q.Set("last_updated__gte", c.since)
	}
	header := map[string][]string{"Authorization": {"Token " + s.secret()}, "Accept": {"application/json"}}
	var answer struct {
		Results []map[string]any `json:"results"`
	}
	if _, err := s.getJSON(ctx, s.base("/api/dcim/devices/?"+q.Encode()), header, &answer); err != nil {
		return port.Page{}, err
	}
	var page port.Page
	for _, r := range answer.Results {
		id, name := text(r["id"]), text(r["name"])
		if id == "" || name == "" {
			continue
		}
		class := "network_device"
		if role, _ := r["role"].(map[string]any); strings.Contains(strings.ToLower(text(role["slug"])), "server") {
			class = "server"
		}
		var manufacturer, model string
		if dt, ok := r["device_type"].(map[string]any); ok {
			manufacturer, model = text(dt["manufacturer"]), text(dt["model"])
		}
		var address string
		if ip, ok := r["primary_ip"].(map[string]any); ok {
			address, _, _ = strings.Cut(text(ip["address"]), "/")
		}
		c.saw(text(r["last_updated"]))
		page.Payload.Items = append(page.Payload.Items, cmdb.Item{
			Ref: "netbox-" + id, Class: class, Name: name, NativeKey: id,
			Attributes: attrs("hostname", name, "serial_number", text(r["serial"]), "asset_tag", text(r["asset_tag"]),
				"manufacturer", manufacturer, "model", model, "ip_address", address),
		})
	}
	page.Cursor, page.More = c.next(len(answer.Results), s.size)
	return page, nil
}

// microsoft answers a Graph token for the directory the connector names.
func (s session) microsoft(ctx context.Context) (string, error) {
	login := strings.TrimRight(s.setting("authority"), "/")
	return s.token(ctx, login+"/"+url.PathEscape(s.setting("tenant"))+"/oauth2/v2.0/token", map[string]string{
		"client_id": s.setting("client_id"), "client_secret": s.secret(), "scope": strings.TrimRight(s.pull.BaseURL, "/") + "/.default",
	})
}

type graphPage struct {
	Value     []map[string]any `json:"value"`
	NextLink  string           `json:"@odata.nextLink"`
	DeltaLink string           `json:"@odata.deltaLink"`
}

// intune pulls Intune's managed devices: every device on every walk, since Graph offers no
// delta for them; the cursor is the next page's link while a walk is under way.
func intune(ctx context.Context, s session) (port.Page, error) {
	token, err := s.microsoft(ctx)
	if err != nil {
		return port.Page{}, err
	}
	next := s.pull.Cursor
	if next == "" {
		next = s.base("/v1.0/deviceManagement/managedDevices?$top=" + strconv.Itoa(s.size) +
			"&$select=id,deviceName,serialNumber,manufacturer,model,operatingSystem,osVersion,ethernetMacAddress,wiFiMacAddress,chassisType")
	}
	var answer graphPage
	if _, err := s.getJSON(ctx, next, bearer(token), &answer); err != nil {
		return port.Page{}, err
	}
	var page port.Page
	for _, r := range answer.Value {
		id, name := text(r["id"]), text(r["deviceName"])
		if id == "" || name == "" {
			continue
		}
		class := "workstation"
		if strings.EqualFold(text(r["chassisType"]), "laptop") {
			class = "laptop"
		}
		mac := text(r["ethernetMacAddress"])
		if mac == "" {
			mac = text(r["wiFiMacAddress"])
		}
		page.Payload.Items = append(page.Payload.Items, cmdb.Item{
			Ref: "intune-" + id, Class: class, Name: name, NativeKey: id,
			Attributes: attrs("hostname", name, "serial_number", text(r["serialNumber"]), "manufacturer", text(r["manufacturer"]),
				"model", text(r["model"]), "operating_system", strings.TrimSpace(text(r["operatingSystem"])+" "+text(r["osVersion"])),
				"mac_address", mac),
		})
	}
	page.Cursor, page.More = answer.NextLink, answer.NextLink != ""
	return page, nil
}

// entra pulls Entra ID's devices by delta query: the cursor is the next page's link during a
// walk and the delta link after it, so that each later walk fetches only what changed. A device
// the directory removed is left alone - removing a CI is never a source's to decide.
func entra(ctx context.Context, s session) (port.Page, error) {
	token, err := s.microsoft(ctx)
	if err != nil {
		return port.Page{}, err
	}
	next := s.pull.Cursor
	if next == "" {
		next = s.base("/v1.0/devices/delta?$select=id,deviceId,displayName,operatingSystem,operatingSystemVersion")
	}
	var answer graphPage
	if _, err := s.getJSON(ctx, next, bearer(token), &answer); err != nil {
		return port.Page{}, err
	}
	var page port.Page
	for _, r := range answer.Value {
		if _, removed := r["@removed"]; removed {
			continue
		}
		id, name := text(r["deviceId"]), text(r["displayName"])
		if id == "" {
			id = text(r["id"])
		}
		if id == "" || name == "" {
			continue
		}
		page.Payload.Items = append(page.Payload.Items, cmdb.Item{
			Ref: "entra-" + id, Class: "workstation", Name: name, NativeKey: id,
			Attributes: attrs("hostname", name,
				"operating_system", strings.TrimSpace(text(r["operatingSystem"])+" "+text(r["operatingSystemVersion"]))),
		})
	}
	if answer.NextLink != "" {
		page.Cursor, page.More = answer.NextLink, true
	} else {
		page.Cursor = answer.DeltaLink
	}
	return page, nil
}

// hetzner pulls a Hetzner Cloud project's servers with the project's token, page by page, every
// server on every walk. A server is a virtual machine named by its instance identifier.
func hetzner(ctx context.Context, s session) (port.Page, error) {
	pageNo := 1
	if n, err := strconv.Atoi(s.pull.Cursor); err == nil && n > 0 {
		pageNo = n
	}
	size := min(s.size, 50)
	var answer struct {
		Servers []map[string]any `json:"servers"`
		Meta    struct {
			Pagination struct {
				NextPage *int `json:"next_page"`
			} `json:"pagination"`
		} `json:"meta"`
	}
	q := url.Values{"page": {strconv.Itoa(pageNo)}, "per_page": {strconv.Itoa(size)}}
	if _, err := s.getJSON(ctx, s.base("/v1/servers?"+q.Encode()), bearer(s.secret()), &answer); err != nil {
		return port.Page{}, err
	}
	var page port.Page
	for _, r := range answer.Servers {
		id, name := text(r["id"]), text(r["name"])
		if id == "" || name == "" {
			continue
		}
		var address, system string
		if net, ok := r["public_net"].(map[string]any); ok {
			if v4, ok := net["ipv4"].(map[string]any); ok {
				address = text(v4["ip"])
			}
		}
		if img, ok := r["image"].(map[string]any); ok {
			system = strings.TrimSpace(text(img["os_flavor"]) + " " + text(img["os_version"]))
		}
		page.Payload.Items = append(page.Payload.Items, cmdb.Item{
			Ref: "hetzner-" + id, Class: "virtual_machine", Name: name, NativeKey: id,
			Attributes: attrs("instance_id", "hetzner:"+id, "hostname", name, "ip_address", address, "operating_system", system),
		})
	}
	if next := answer.Meta.Pagination.NextPage; next != nil && *next > pageNo {
		page.Cursor, page.More = strconv.Itoa(*next), true
	}
	return page, nil
}

// openstack pulls an OpenStack project's servers: a Keystone token for an application
// credential, the compute endpoint from its catalogue (a host the connector must allow), then
// the servers page by page by marker, every server on every walk.
func openstack(ctx context.Context, s session) (port.Page, error) {
	body, err := json.Marshal(map[string]any{"auth": map[string]any{"identity": map[string]any{
		"methods":                []string{"application_credential"},
		"application_credential": map[string]string{"id": s.setting("application_credential_id"), "secret": s.secret()},
	}}})
	if err != nil {
		return port.Page{}, unavailable(port.CodeMalformed)
	}
	resp, err := s.call(ctx, "POST", s.base("/v3/auth/tokens"), map[string][]string{"Content-Type": {"application/json"}}, body)
	if err != nil {
		return port.Page{}, err
	}
	token := ""
	for k, v := range resp.Header {
		if strings.EqualFold(k, "X-Subject-Token") && len(v) > 0 {
			token = v[0]
		}
	}
	var auth struct {
		Token struct {
			Catalog []struct {
				Type      string `json:"type"`
				Endpoints []struct {
					Interface string `json:"interface"`
					Region    string `json:"region"`
					URL       string `json:"url"`
				} `json:"endpoints"`
			} `json:"catalog"`
		} `json:"token"`
	}
	if err := decode(resp.Body, &auth); err != nil || token == "" {
		return port.Page{}, unavailable(port.CodeUnauthorised)
	}
	compute, region := "", s.setting("region")
	for _, svc := range auth.Token.Catalog {
		if svc.Type != "compute" {
			continue
		}
		for _, e := range svc.Endpoints {
			if e.Interface == "public" && (region == "" || e.Region == region) {
				compute = strings.TrimRight(e.URL, "/")
			}
		}
	}
	if compute == "" {
		return port.Page{}, unavailable(port.CodeMalformed)
	}
	q := url.Values{"limit": {strconv.Itoa(s.size)}}
	if s.pull.Cursor != "" {
		q.Set("marker", s.pull.Cursor)
	}
	var answer struct {
		Servers []map[string]any `json:"servers"`
	}
	if _, err := s.getJSON(ctx, compute+"/servers/detail?"+q.Encode(),
		map[string][]string{"X-Auth-Token": {token}, "Accept": {"application/json"}}, &answer); err != nil {
		return port.Page{}, err
	}
	var page port.Page
	last := ""
	for _, r := range answer.Servers {
		id, name := text(r["id"]), text(r["name"])
		if id == "" {
			continue
		}
		last = id
		if name == "" {
			continue
		}
		page.Payload.Items = append(page.Payload.Items, cmdb.Item{
			Ref: "openstack-" + id, Class: "virtual_machine", Name: name, NativeKey: id,
			Attributes: attrs("instance_id", "openstack:"+id, "hostname", name, "ip_address", firstAddress(r["addresses"])),
		})
	}
	if len(answer.Servers) >= s.size && last != "" {
		page.Cursor, page.More = last, true
	}
	return page, nil
}

// firstAddress answers the first address of a server's networks, in the networks' name order.
func firstAddress(v any) string {
	nets, _ := v.(map[string]any)
	best := ""
	bestNet := ""
	for net, list := range nets {
		addrs, _ := list.([]any)
		for _, a := range addrs {
			m, _ := a.(map[string]any)
			if addr := text(m["addr"]); addr != "" && (bestNet == "" || net < bestNet) {
				best, bestNet = addr, net
				break
			}
		}
	}
	return best
}
