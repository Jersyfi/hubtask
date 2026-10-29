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

// netbox pulls NetBox's devices, then its virtual machines, each with a token and changed since
// the newest the last walk of it saw. A device whose role names a server is a server, any other a
// network device; its primary address, without the prefix length, is its address. A device and a
// virtual machine are contained in their site, and a virtual machine is hosted on its cluster and
// on the device it runs on. Sites and clusters come along as the records name them; their keys,
// like a virtual machine's, carry their kind, since NetBox numbers each kind on its own.
func netbox(ctx context.Context, s session) (port.Page, error) {
	c := readNetBox(s.pull.Cursor)
	walk, path := c.devices, "/api/dcim/devices/"
	if c.machines {
		walk, path = c.vms, "/api/virtualization/virtual-machines/"
	}
	o := readOffset(walk)
	q := url.Values{"limit": {strconv.Itoa(s.size)}, "offset": {strconv.Itoa(o.offset)}, "ordering": {"last_updated"}}
	if o.since != "" {
		q.Set("last_updated__gte", o.since)
	}
	header := map[string][]string{"Authorization": {"Token " + s.secret()}, "Accept": {"application/json"}}
	var answer struct {
		Results []map[string]any `json:"results"`
	}
	if _, err := s.getJSON(ctx, s.base(path+"?"+q.Encode()), header, &answer); err != nil {
		return port.Page{}, err
	}
	b := payloadBuilder{refs: map[string]bool{}}
	for _, r := range answer.Results {
		id, name := text(r["id"]), text(r["name"])
		if id == "" || name == "" {
			continue
		}
		o.saw(text(r["last_updated"]))
		ref := "netbox-" + id
		if c.machines {
			ref = "netbox-vm-" + id
			b.add(cmdb.Item{Ref: ref, Class: "virtual_machine", Name: name, NativeKey: "vm:" + id,
				Attributes: attrs("instance_id", "netbox:"+id, "hostname", name, "ip_address", primaryAddress(r),
					"operating_system", text(r["platform"]))})
			if cl, ok := r["cluster"].(map[string]any); ok && text(cl["id"]) != "" && text(cl["name"]) != "" {
				to := "netbox-cluster-" + text(cl["id"])
				b.add(cmdb.Item{Ref: to, Class: "virtualization_cluster", Name: text(cl["name"]), NativeKey: "cluster:" + text(cl["id"]),
					Attributes: attrs("cluster_id", "netbox:"+text(cl["id"]))})
				b.relate(cmdb.RelHostedOn, ref, to)
			}
			if dev, ok := r["device"].(map[string]any); ok && text(dev["id"]) != "" && text(dev["name"]) != "" {
				to := "netbox-" + text(dev["id"])
				b.add(cmdb.Item{Ref: to, Class: "server", Name: text(dev["name"]), NativeKey: text(dev["id"])})
				b.relate(cmdb.RelHostedOn, ref, to)
			}
		} else {
			class := "network_device"
			if role, _ := r["role"].(map[string]any); strings.Contains(strings.ToLower(text(role["slug"])), "server") {
				class = "server"
			}
			var manufacturer, model string
			if dt, ok := r["device_type"].(map[string]any); ok {
				manufacturer, model = text(dt["manufacturer"]), text(dt["model"])
			}
			b.add(cmdb.Item{Ref: ref, Class: class, Name: name, NativeKey: id,
				Attributes: attrs("hostname", name, "serial_number", text(r["serial"]), "asset_tag", text(r["asset_tag"]),
					"manufacturer", manufacturer, "model", model, "ip_address", primaryAddress(r))})
		}
		if site, ok := r["site"].(map[string]any); ok && text(site["id"]) != "" && text(site["name"]) != "" {
			from := "netbox-site-" + text(site["id"])
			b.add(cmdb.Item{Ref: from, Class: "site", Name: text(site["name"]), NativeKey: "site:" + text(site["id"])})
			b.relate(cmdb.RelContains, from, ref)
		}
	}
	next, more := o.next(len(answer.Results), s.size)
	page := port.Page{Payload: b.payload}
	switch {
	case more && c.machines:
		c.vms = next
	case more:
		c.devices = next
	case c.machines:
		// Both walks are done: the next pull starts the devices' walk since their newest change.
		c.vms, c.machines = next, false
	default:
		c.devices, c.machines = next, true
		more = true
	}
	page.Cursor, page.More = c.String(), more
	return page, nil
}

// primaryAddress answers a NetBox record's primary address without its prefix length.
func primaryAddress(r map[string]any) string {
	ip, ok := r["primary_ip"].(map[string]any)
	if !ok {
		return ""
	}
	address, _, _ := strings.Cut(text(ip["address"]), "/")
	return address
}

// netboxCursor is where the devices' and the virtual machines' walks stand and which is under
// way. A cursor without the separator is a devices' walk's alone, as pulls before the virtual
// machines wrote it.
type netboxCursor struct {
	machines     bool
	devices, vms string
}

func readNetBox(raw string) netboxCursor {
	parts := strings.Split(raw, "#")
	if len(parts) != 3 {
		return netboxCursor{devices: raw}
	}
	return netboxCursor{machines: parts[0] == "vm", devices: parts[1], vms: parts[2]}
}

func (c netboxCursor) String() string {
	phase := "dev"
	if c.machines {
		phase = "vm"
	}
	return phase + "#" + c.devices + "#" + c.vms
}

// payloadBuilder gathers a page's items once each by their reference, and the relations between
// them, within the reconciliation's bounds.
type payloadBuilder struct {
	payload cmdb.Payload
	refs    map[string]bool
}

func (b *payloadBuilder) add(item cmdb.Item) bool {
	if b.refs[item.Ref] {
		return true
	}
	if len(b.payload.Items) >= cmdb.MaxPayloadItems {
		return false
	}
	b.refs[item.Ref] = true
	b.payload.Items = append(b.payload.Items, item)
	return true
}

func (b *payloadBuilder) relate(kind, from, to string) {
	if b.refs[from] && b.refs[to] && len(b.payload.Relations) < cmdb.MaxPayloadRelations {
		b.payload.Relations = append(b.payload.Relations, cmdb.RelationItem{Type: kind, From: from, To: to})
	}
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

// intune pulls Intune's managed devices, then the apps Intune detected on them: every device and
// app on every walk, since Graph offers no delta for them; the cursor is the next page's link
// while the devices' walk is under way, then where the apps' walk stands. The setting
// `detected_apps` set to `off` leaves the apps out.
func intune(ctx context.Context, s session) (port.Page, error) {
	token, err := s.microsoft(ctx)
	if err != nil {
		return port.Page{}, err
	}
	if c, ok := readApps(s.pull.Cursor); ok {
		return detectedApps(ctx, s, token, c)
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
	if !page.More && !strings.EqualFold(s.setting("detected_apps"), "off") {
		page.Cursor, page.More = appsCursor{}.String(), true
	}
	return page, nil
}

// appsPerPull bounds one pull of detected apps, so that a page with each app's devices stays
// within the reconciliation's bounds (MaxPayloadItems).
const appsPerPull = 10

// appsCursor is where the walk through Intune's detected apps stands: the page of apps, the app
// within it and the next page of that app's devices.
type appsCursor struct {
	apps    string
	index   int
	devices string
}

const appsPrefix = "apps\n"

func readApps(raw string) (appsCursor, bool) {
	rest, ok := strings.CutPrefix(raw, appsPrefix)
	if !ok {
		return appsCursor{}, false
	}
	parts := strings.Split(rest, "\n")
	c := appsCursor{apps: parts[0]}
	if len(parts) > 1 {
		c.index, _ = strconv.Atoi(parts[1])
	}
	if len(parts) > 2 {
		c.devices = parts[2]
	}
	return c, true
}

func (c appsCursor) String() string {
	return appsPrefix + c.apps + "\n" + strconv.Itoa(c.index) + "\n" + c.devices
}

// detectedApps pulls a page of Intune's detected apps, each as software - one CI per app and
// version, Graph's identifier its native key - installed on every managed device it was
// detected on. An app with more devices than one page ends the pull at that app.
func detectedApps(ctx context.Context, s session, token string, c appsCursor) (port.Page, error) {
	if c.apps == "" {
		c.apps = s.base("/v1.0/deviceManagement/detectedApps?$top=" + strconv.Itoa(appsPerPull) + "&$select=id,displayName,version,publisher")
	}
	var apps graphPage
	if _, err := s.getJSON(ctx, c.apps, bearer(token), &apps); err != nil {
		return port.Page{}, err
	}
	b := payloadBuilder{refs: map[string]bool{}}
	end := min(len(apps.Value), c.index+appsPerPull)
	for i := c.index; i < end; i++ {
		r := apps.Value[i]
		id, name := text(r["id"]), strings.TrimSpace(text(r["displayName"]))
		if id == "" || name == "" {
			continue
		}
		link := s.base("/v1.0/deviceManagement/detectedApps/" + url.PathEscape(id) + "/managedDevices?$select=id,deviceName&$top=" + strconv.Itoa(s.size))
		if i == c.index && c.devices != "" {
			link = c.devices
		}
		var devices graphPage
		if _, err := s.getJSON(ctx, link, bearer(token), &devices); err != nil {
			return port.Page{}, err
		}
		app := "intune-app-" + id
		b.add(cmdb.Item{Ref: app, Class: "software", Name: name, NativeKey: "app:" + id,
			Attributes: attrs("version", text(r["version"]), "publisher", text(r["publisher"]))})
		for _, d := range devices.Value {
			device, deviceName := text(d["id"]), text(d["deviceName"])
			if device == "" || deviceName == "" {
				continue
			}
			if b.add(cmdb.Item{Ref: "intune-" + device, Class: "workstation", Name: deviceName, NativeKey: device}) {
				b.relate(cmdb.RelInstalled, app, "intune-"+device)
			}
		}
		if devices.NextLink != "" {
			c.index, c.devices = i, devices.NextLink
			return port.Page{Payload: b.payload, Cursor: c.String(), More: true}, nil
		}
	}
	switch {
	case end < len(apps.Value):
		c.index, c.devices = end, ""
	case apps.NextLink != "":
		c = appsCursor{apps: apps.NextLink}
	default:
		// The walk is done; the next starts at the devices again.
		return port.Page{Payload: b.payload}, nil
	}
	return port.Page{Payload: b.payload, Cursor: c.String(), More: true}, nil
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
