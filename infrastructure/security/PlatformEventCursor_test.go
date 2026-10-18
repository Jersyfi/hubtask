// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package security

import (
	"strings"
	"testing"
	"time"

	adminrepo "github.com/Jersyfi/hubtask/core/application/repository/admin"
	"github.com/Jersyfi/hubtask/core/application/repository/outbox"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

func TestAPlatformCursorRoundTripsAndRefusesAForgery(t *testing.T) {
	codec := NewPlatformEventCursorCodec(secret.New("0123456789abcdef0123456789abcdef"))
	position := adminrepo.PlatformPosition{Number: 4711, At: time.Date(2026, 10, 17, 9, 0, 0, 123000, time.UTC)}

	read, err := codec.Decode(codec.Encode(position))
	if err != nil || read.Number != position.Number || !read.At.Equal(position.At) {
		t.Fatalf("read back %+v, %v", read, err)
	}

	other := NewPlatformEventCursorCodec(secret.New("fedcba9876543210fedcba9876543210"))
	trigger := NewTriggerCursorCodec(secret.New("0123456789abcdef0123456789abcdef"))
	encoded := codec.Encode(position)
	for name, cursor := range map[string]string{
		"another installation's": other.Encode(position),
		"truncated":              encoded[:len(encoded)-2],
		"not base64":             "!!!",
		"empty":                  "",
		"tampered":               strings.ToUpper(encoded[:3]) + encoded[3:],
		"a trigger cursor":       trigger.Encode(outbox.Position{OccurredAt: position.At}),
	} {
		if _, err := codec.Decode(cursor); err == nil {
			t.Errorf("%s cursor was read", name)
		}
	}
}
