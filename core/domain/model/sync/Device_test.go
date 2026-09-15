// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package sync_test

import (
	"strings"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/sync"
)

var (
	v7 = shared.MustParseID("0192f000-0000-7000-8000-0000000000d1")
	v4 = shared.MustParseID("0192f000-0000-4000-8000-0000000000d1")
)

func TestAContactIsBoundedAndNormalised(t *testing.T) {
	cases := map[string]struct {
		contact sync.Contact
		code    string
		want    sync.Contact
	}{
		"a device is required": {
			contact: sync.Contact{}, code: "sync.device_required",
		},
		"the identifier has to be one the client minted": {
			contact: sync.Contact{DeviceID: v4}, code: "sync.id_not_uuidv7",
		},
		"the platform is bounded": {
			contact: sync.Contact{DeviceID: v7, Platform: strings.Repeat("x", sync.PlatformMaxLength+1)},
			code:    "sync.device_platform_too_long",
		},
		"the name is bounded": {
			contact: sync.Contact{DeviceID: v7, DisplayName: strings.Repeat("x", sync.DisplayNameMaxLength+1)},
			code:    "sync.device_name_too_long",
		},
		"what it says is trimmed": {
			contact: sync.Contact{DeviceID: v7, Platform: "  ios ", DisplayName: " Anna's phone "},
			want:    sync.Contact{DeviceID: v7, Platform: "ios", DisplayName: "Anna's phone"},
		},
		"saying nothing is allowed": {
			contact: sync.Contact{DeviceID: v7}, want: sync.Contact{DeviceID: v7},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := tc.contact.Validate()
			if tc.code != "" {
				if err == nil {
					t.Fatalf("accepted %+v", tc.contact)
				}
				if code := shared.AsError(err).DetailCode; code != tc.code {
					t.Errorf("refused with %q, want %q", code, tc.code)
				}
				return
			}
			if err != nil {
				t.Fatalf("refused: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}
