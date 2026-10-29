// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package connector_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/cmdb"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	port "github.com/Jersyfi/hubtask/core/port/connector"
	env "github.com/Jersyfi/hubtask/core/port/environment"
	"github.com/Jersyfi/hubtask/core/shared/secret"
	"github.com/Jersyfi/hubtask/infrastructure/connector"
	"github.com/Jersyfi/hubtask/infrastructure/httpclient"
)

// Every fake answers on loopback, which the guard refuses by default: the tests open private
// networks as a self-hoster with an internal source does, and the calls still go through the
// guarded client (rule 6).
func guarded(t *testing.T) *httpclient.GuardedClient {
	t.Helper()
	cfg := env.OutboundConfig{AllowPrivateNetworks: true, Timeout: 2 * time.Second, ConnectTimeout: time.Second, MaxResponseBytes: 1 << 20}
	return httpclient.NewGuardedClient(cfg, httpclient.NewGuard(cfg))
}

func pullers(t *testing.T, server *httptest.Server) connector.Pullers {
	return connector.Pullers{Client: guarded(t), PageSize: 2}
}

func pullOf(server *httptest.Server, kind cmdb.ConnectorKind, settings map[string]string, cursor string) port.Pull {
	return port.Pull{
		Connector: shared.ID("00000000-0000-4000-8000-000000000001"), Kind: kind, BaseURL: server.URL,
		Settings: settings, Hosts: []string{"127.0.0.1"}, Credential: secret.New("s3cret"), Cursor: cursor,
	}
}

func detail(err error) string {
	var e *shared.Error
	if errors.As(err, &e) {
		return e.DetailCode
	}
	return ""
}

func TestGLPIIsPulledWithAnOAuthClientSinceTheNewestChange(t *testing.T) {
	var asked []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api.php/token":
			body, _ := io.ReadAll(r.Body)
			if !strings.Contains(string(body), "client_secret=s3cret") || !strings.Contains(string(body), "grant_type=client_credentials") {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			_, _ = w.Write([]byte(`{"access_token":"t1"}`))
		case "/api.php/v2/Assets/Computer":
			if r.Header.Get("Authorization") != "Bearer t1" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			asked = append(asked, r.URL.RawQuery)
			_, _ = w.Write([]byte(`[{"id":7,"name":"srv-1","serial":"S1","type":{"name":"Server"},"manufacturer":{"name":"Dell"},"date_mod":"2026-10-29 10:00:00"},
				{"id":8,"name":"pc-2","otherserial":"A-2","date_mod":"2026-10-29 11:00:00"}]`))
		}
	}))
	defer server.Close()
	page, err := pullers(t, server).Pull(context.Background(), pullOf(server, cmdb.ConnectorGLPI, map[string]string{"client_id": "c"}, ""))
	if err != nil {
		t.Fatal(err)
	}
	items := page.Payload.Items
	if len(items) != 2 || items[0].Class != "server" || items[0].NativeKey != "7" || items[0].Attributes["manufacturer"] != "Dell" ||
		items[1].Class != "workstation" || items[1].Attributes["asset_tag"] != "A-2" {
		t.Fatalf("items %+v", items)
	}
	if !page.More || page.Cursor != "|2|2026-10-29 11:00:00" {
		t.Fatalf("a full page asks for the next: %q %v", page.Cursor, page.More)
	}
	if _, err := pullers(t, server).Pull(context.Background(), pullOf(server, cmdb.ConnectorGLPI, map[string]string{"client_id": "c"}, "2026-10-29 09:00:00|0|")); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(asked[1], "filter=date_mod%3Dge%3D2026-10-29+09%3A00%3A00") {
		t.Fatalf("a later walk asks since the cursor: %q", asked[1])
	}
}

func TestNetBoxDevicesBecomeServersAndNetworkDevices(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/dcim/devices/" || r.Header.Get("Authorization") != "Token s3cret" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		_, _ = w.Write([]byte(`{"count":1,"results":[{"id":3,"name":"sw-1","serial":"X","role":{"slug":"access-switch"},"site":{"id":1,"name":"Frankfurt"},
			"device_type":{"manufacturer":{"name":"Cisco"},"model":"C9200"},"primary_ip":{"address":"10.0.0.2/24"},"last_updated":"2026-10-29T10:00:00Z"}]}`))
	}))
	defer server.Close()
	page, err := pullers(t, server).Pull(context.Background(), pullOf(server, cmdb.ConnectorNetBox, nil, ""))
	if err != nil {
		t.Fatal(err)
	}
	it := page.Payload.Items[0]
	if it.Class != "network_device" || it.Attributes["ip_address"] != "10.0.0.2" || it.Attributes["model"] != "C9200" {
		t.Fatalf("item %+v", it)
	}
	if len(page.Payload.Items) != 2 || page.Payload.Items[1].Class != "site" || page.Payload.Items[1].NativeKey != "site:1" ||
		len(page.Payload.Relations) != 1 || page.Payload.Relations[0] != (cmdb.RelationItem{Type: cmdb.RelContains, From: "netbox-site-1", To: "netbox-3"}) {
		t.Fatalf("the device's site contains it: %+v", page.Payload)
	}
	if !page.More || page.Cursor != "vm#2026-10-29T10:00:00Z#" {
		t.Fatalf("a short page ends the devices' walk at the newest change and starts the machines': %q %v", page.Cursor, page.More)
	}
}

func TestNetBoxVirtualMachinesAreHostedOnTheirClusterAndDevice(t *testing.T) {
	var asked string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/virtualization/virtual-machines/" || r.Header.Get("Authorization") != "Token s3cret" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		asked = r.URL.RawQuery
		_, _ = w.Write([]byte(`{"count":2,"results":[
			{"id":3,"name":"app-1","cluster":{"id":5,"name":"vsphere-a"},"device":{"id":3,"name":"esx-1"},"site":{"id":1,"name":"Frankfurt"},
			 "platform":{"name":"Ubuntu 24.04"},"primary_ip":{"address":"10.2.0.7/24"},"last_updated":"2026-10-29T12:00:00Z"},
			{"id":4,"name":"app-2","cluster":{"id":5,"name":"vsphere-a"},"last_updated":"2026-10-29T13:00:00Z"}]}`))
	}))
	defer server.Close()
	page, err := pullers(t, server).Pull(context.Background(), pullOf(server, cmdb.ConnectorNetBox, nil, "vm#2026-10-29T10:00:00Z#2026-10-28T00:00:00Z"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(asked, "last_updated__gte=2026-10-28T00%3A00%3A00Z") {
		t.Fatalf("the machines' walk asks since its own newest change: %q", asked)
	}
	byRef := map[string]cmdb.Item{}
	for _, it := range page.Payload.Items {
		byRef[it.Ref] = it
	}
	vm := byRef["netbox-vm-3"]
	if len(page.Payload.Items) != 5 || vm.Class != "virtual_machine" || vm.NativeKey != "vm:3" || vm.Attributes["instance_id"] != "netbox:3" ||
		vm.Attributes["ip_address"] != "10.2.0.7" || vm.Attributes["operating_system"] != "Ubuntu 24.04" ||
		byRef["netbox-cluster-5"].Class != "virtualization_cluster" || byRef["netbox-3"].NativeKey != "3" || byRef["netbox-site-1"].Class != "site" {
		t.Fatalf("items %+v", page.Payload.Items)
	}
	want := []cmdb.RelationItem{
		{Type: cmdb.RelHostedOn, From: "netbox-vm-3", To: "netbox-cluster-5"},
		{Type: cmdb.RelHostedOn, From: "netbox-vm-3", To: "netbox-3"},
		{Type: cmdb.RelContains, From: "netbox-site-1", To: "netbox-vm-3"},
		{Type: cmdb.RelHostedOn, From: "netbox-vm-4", To: "netbox-cluster-5"},
	}
	if len(page.Payload.Relations) != len(want) {
		t.Fatalf("relations %+v", page.Payload.Relations)
	}
	for i := range want {
		if page.Payload.Relations[i] != want[i] {
			t.Fatalf("relation %d: %+v", i, page.Payload.Relations[i])
		}
	}
	if !page.More || page.Cursor != "vm#2026-10-29T10:00:00Z#2026-10-28T00:00:00Z|2|2026-10-29T13:00:00Z" {
		t.Fatalf("a full page asks for the next: %q %v", page.Cursor, page.More)
	}
}

func TestNetBoxWalksEndAfterTheMachinesAndStartAgainAtTheDevices(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"count":0,"results":[]}`))
	}))
	defer server.Close()
	page, err := pullers(t, server).Pull(context.Background(), pullOf(server, cmdb.ConnectorNetBox, nil, "vm#2026-10-29T10:00:00Z#2026-10-28T00:00:00Z"))
	if err != nil || page.More || page.Cursor != "dev#2026-10-29T10:00:00Z#2026-10-28T00:00:00Z" {
		t.Fatalf("the walk ends: %q %v %v", page.Cursor, page.More, err)
	}
	page, err = pullers(t, server).Pull(context.Background(), pullOf(server, cmdb.ConnectorNetBox, nil, "2026-10-27T00:00:00Z"))
	if err != nil || !page.More || page.Cursor != "vm#2026-10-27T00:00:00Z#" {
		t.Fatalf("a devices' cursor of before goes on to the machines: %q %v %v", page.Cursor, page.More, err)
	}
}

func microsoftServer(t *testing.T, graph func(w http.ResponseWriter, r *http.Request)) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/dir-1/oauth2/v2.0/token" {
			body, _ := io.ReadAll(r.Body)
			if !strings.Contains(string(body), "client_id=app") || !strings.Contains(string(body), "%2F.default") {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			_, _ = w.Write([]byte(`{"access_token":"g1"}`))
			return
		}
		if r.Header.Get("Authorization") != "Bearer g1" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		graph(w, r)
	}))
}

func TestIntuneDevicesArePulledPageByPage(t *testing.T) {
	var server *httptest.Server
	server = microsoftServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("page") == "2" {
			_, _ = w.Write([]byte(`{"value":[{"id":"d2","deviceName":"lap-2","chassisType":"laptop","wiFiMacAddress":"AA:BB"}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"value":[{"id":"d1","deviceName":"pc-1","serialNumber":"SN1","operatingSystem":"Windows","osVersion":"11"}],
			"@odata.nextLink":"` + server.URL + `/v1.0/deviceManagement/managedDevices?page=2"}`))
	})
	defer server.Close()
	settings := map[string]string{"tenant": "dir-1", "client_id": "app", "authority": server.URL}
	page, err := pullers(t, server).Pull(context.Background(), pullOf(server, cmdb.ConnectorIntune, settings, ""))
	if err != nil {
		t.Fatal(err)
	}
	if !page.More || page.Payload.Items[0].Attributes["operating_system"] != "Windows 11" {
		t.Fatalf("first page %+v", page)
	}
	page, err = pullers(t, server).Pull(context.Background(), pullOf(server, cmdb.ConnectorIntune, settings, page.Cursor))
	if err != nil || page.More || page.Payload.Items[0].Class != "laptop" || page.Payload.Items[0].Attributes["mac_address"] != "AA:BB" {
		t.Fatalf("second page %+v %v", page, err)
	}
}

func TestEntraDevicesEndAtADeltaLinkAndSkipRemovedOnes(t *testing.T) {
	var server *httptest.Server
	server = microsoftServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"value":[{"id":"o1","deviceId":"d1","displayName":"pc-1"},{"id":"o2","@removed":{"reason":"deleted"}}],
			"@odata.deltaLink":"` + server.URL + `/v1.0/devices/delta?$deltatoken=abc"}`))
	})
	defer server.Close()
	page, err := pullers(t, server).Pull(context.Background(), pullOf(server, cmdb.ConnectorEntra, map[string]string{"tenant": "dir-1", "client_id": "app", "authority": server.URL}, ""))
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Payload.Items) != 1 || page.Payload.Items[0].NativeKey != "d1" || page.More || !strings.HasSuffix(page.Cursor, "$deltatoken=abc") {
		t.Fatalf("page %+v", page)
	}
}

func TestHetznerServersAreVirtualMachinesAndARateLimitIsHonoured(t *testing.T) {
	limited := true
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if limited {
			w.Header().Set("Retry-After", "120")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		_, _ = w.Write([]byte(`{"servers":[{"id":42,"name":"web-1","public_net":{"ipv4":{"ip":"203.0.113.5"}},"image":{"os_flavor":"debian","os_version":"12"}}],
			"meta":{"pagination":{"next_page":null}}}`))
	}))
	defer server.Close()
	_, err := pullers(t, server).Pull(context.Background(), pullOf(server, cmdb.ConnectorHetzner, nil, ""))
	if detail(err) != port.CodeRateLimited || port.RetryAfter(err) != 2*time.Minute {
		t.Fatalf("a 429 answers the wait asked for: %v", err)
	}
	limited = false
	page, err := pullers(t, server).Pull(context.Background(), pullOf(server, cmdb.ConnectorHetzner, nil, ""))
	if err != nil {
		t.Fatal(err)
	}
	it := page.Payload.Items[0]
	if it.Class != "virtual_machine" || it.Attributes["instance_id"] != "hetzner:42" || it.Attributes["ip_address"] != "203.0.113.5" || page.More {
		t.Fatalf("item %+v", it)
	}
}

func TestOpenStackServersAreReadFromTheCataloguesComputeEndpoint(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v3/auth/tokens":
			body, _ := io.ReadAll(r.Body)
			if !strings.Contains(string(body), `"secret":"s3cret"`) {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			w.Header().Set("X-Subject-Token", "ks1")
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"token":{"catalog":[{"type":"compute","endpoints":[{"interface":"public","region":"r1","url":"` + server.URL + `/compute/v2.1"}]}]}}`))
		case "/compute/v2.1/servers/detail":
			if r.Header.Get("X-Auth-Token") != "ks1" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			_, _ = w.Write([]byte(`{"servers":[{"id":"u1","name":"db-1","addresses":{"private":[{"addr":"10.1.0.4"}]}},{"id":"u2","name":"db-2"}]}`))
		}
	}))
	defer server.Close()
	page, err := pullers(t, server).Pull(context.Background(), pullOf(server, cmdb.ConnectorOpenStack, map[string]string{"application_credential_id": "ac"}, ""))
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Payload.Items) != 2 || page.Payload.Items[0].Attributes["ip_address"] != "10.1.0.4" || !page.More || page.Cursor != "u2" {
		t.Fatalf("page %+v", page)
	}
}

func TestAPullReachesNoHostTheConnectorDoesNotAllow(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("a host not allowed was called")
	}))
	defer server.Close()
	pull := pullOf(server, cmdb.ConnectorHetzner, nil, "")
	pull.Hosts = []string{"api.hetzner.cloud"}
	if _, err := pullers(t, server).Pull(context.Background(), pull); detail(err) != port.CodeHostNotAllowed {
		t.Fatalf("got %v", err)
	}
	pull.Hosts, pull.Credential = []string{"127.0.0.1"}, secret.Secret{}
	if _, err := pullers(t, server).Pull(context.Background(), pull); detail(err) != port.CodeNoCredential {
		t.Fatalf("a pull without a credential: %v", err)
	}
}

func TestAWrongCredentialIsUnauthorised(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()
	if _, err := pullers(t, server).Pull(context.Background(), pullOf(server, cmdb.ConnectorNetBox, nil, "")); detail(err) != port.CodeUnauthorised {
		t.Fatalf("got %v", err)
	}
}
