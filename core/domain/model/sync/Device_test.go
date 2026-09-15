// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package sync_test

import (
	"strings"
	"testing"
	"time"

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

func TestAReadingBeyondTheSkewIsBoundedToServerTimeAndReported(t *testing.T) {
	serverNow := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	within, _ := shared.NewHLC(serverNow.Add(2*time.Minute), 3, "dev-a")
	ahead, _ := shared.NewHLC(serverNow.Add(3*time.Hour), 3, "dev-a")
	behind, _ := shared.NewHLC(serverNow.Add(-3*time.Hour), 3, "dev-a")

	if got, drift := sync.Bound(within, serverNow, 5*time.Minute); got != within || drift != 0 {
		t.Errorf("a reading within the skew was changed to %s (drift %v)", got, drift)
	}
	for name, reading := range map[string]shared.HLC{"ahead": ahead, "behind": behind} {
		got, drift := sync.Bound(reading, serverNow, 5*time.Minute)
		if !got.Physical.Equal(serverNow) || got.Counter != 3 || got.Device != "dev-a" {
			t.Errorf("%s: bounded to %s, want server time with the device's counter and identifier", name, got)
		}
		if drift != 3*time.Hour {
			t.Errorf("%s: drift %v, want three hours", name, drift)
		}
	}
}

func TestEveryMutationKindTheContractNamesIsValidAndNothingElseIs(t *testing.T) {
	for _, kind := range sync.MutationKinds() {
		if !kind.Valid() {
			t.Errorf("%s is in the closed set and not valid", kind)
		}
	}
	if sync.MutationKind("ITEM_RENAME").Valid() {
		t.Errorf("a kind the contract does not name is valid")
	}
}
