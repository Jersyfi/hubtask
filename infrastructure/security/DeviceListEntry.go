// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package security

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"strings"

	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// deviceListInfo separates this use of the installation secret from every other, so no cursor or
// token is ever read as an entry of the device's list, nor an entry as one of them.
const deviceListInfo = "hubtask/device-list-entry/v1"

// errDeviceListEntryInvalid is the one refusal for every entry not signed here.
var errDeviceListEntryInvalid = shared.ErrValidation.WithDetail("auth.device_list_entry_invalid")

// DeviceListEntryCodec signs an entry of the device's list of workspaces (ADR-0095 §7) and reads
// one back. The entry is `<payload>.<tag>`, both unpadded base64url: the payload is the JSON of the
// host and the display name and nothing else, readable by a client that draws the list before the
// installation confirmed it; the tag is the HMAC that makes it the installation's.
type DeviceListEntryCodec struct {
	key []byte
}

// NewDeviceListEntryCodec keys the codec on the installation secret.
func NewDeviceListEntryCodec(installationSecret secret.Secret) DeviceListEntryCodec {
	mac := hmac.New(sha256.New, []byte(installationSecret.Reveal()))
	mac.Write([]byte(deviceListInfo))
	return DeviceListEntryCodec{key: mac.Sum(nil)}
}

// deviceListPayload is what an entry says; the names are what a client reads.
type deviceListPayload struct {
	Host string `json:"host"`
	Name string `json:"name"`
}

// Sign answers the entry.
func (c DeviceListEntryCodec) Sign(entry domain.DeviceListEntry) string {
	payload, _ := json.Marshal(deviceListPayload{Host: entry.Host, Name: entry.Name})
	encoded := base64.RawURLEncoding.EncodeToString(payload)
	return encoded + "." + base64.RawURLEncoding.EncodeToString(c.tag(encoded))
}

// Read answers what a signed entry says; every entry not signed here is the same refusal.
func (c DeviceListEntryCodec) Read(signed string) (domain.DeviceListEntry, error) {
	encoded, tagText, found := strings.Cut(signed, ".")
	if !found {
		return domain.DeviceListEntry{}, errDeviceListEntryInvalid
	}
	tag, err := base64.RawURLEncoding.DecodeString(tagText)
	if err != nil || !hmac.Equal(tag, c.tag(encoded)) {
		return domain.DeviceListEntry{}, errDeviceListEntryInvalid
	}
	raw, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return domain.DeviceListEntry{}, errDeviceListEntryInvalid
	}
	var payload deviceListPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return domain.DeviceListEntry{}, errDeviceListEntryInvalid
	}
	entry, ok := domain.NewDeviceListEntry(payload.Host, payload.Name)
	if !ok {
		return domain.DeviceListEntry{}, errDeviceListEntryInvalid
	}
	return entry, nil
}

func (c DeviceListEntryCodec) tag(encoded string) []byte {
	mac := hmac.New(sha256.New, c.key)
	mac.Write([]byte(encoded))
	return mac.Sum(nil)
}
