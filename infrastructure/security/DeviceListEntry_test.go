// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package security

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

func TestADeviceListEntryRoundTripsAndRefusesAForgery(t *testing.T) {
	codec := NewDeviceListEntryCodec(secret.New("0123456789abcdef0123456789abcdef"))
	entry := domain.DeviceListEntry{Host: "acme.hub.example", Name: "Acme Ltd"}
	signed := codec.Sign(entry)

	read, err := codec.Read(signed)
	if err != nil || read != entry {
		t.Fatalf("read back %+v, %v", read, err)
	}

	// The payload says the host and the name, and nothing else.
	payload, _, _ := strings.Cut(signed, ".")
	raw, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err := json.Unmarshal(raw, &fields); err != nil || len(fields) != 2 || fields["host"] != entry.Host || fields["name"] != entry.Name {
		t.Fatalf("payload %s", raw)
	}

	other := NewDeviceListEntryCodec(secret.New("fedcba9876543210fedcba9876543210"))
	planted := base64.RawURLEncoding.EncodeToString([]byte(`{"host":"acme-login.example","name":"Acme Ltd"}`))
	_, tag, _ := strings.Cut(signed, ".")
	for name, forged := range map[string]string{
		"another installation's": other.Sign(entry),
		"another payload":        planted + "." + tag,
		"truncated":              signed[:len(signed)-2],
		"no tag":                 payload,
		"empty":                  "",
	} {
		if _, err := codec.Read(forged); err == nil {
			t.Errorf("%s entry was read", name)
		}
	}
}
