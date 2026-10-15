// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"testing"

	envport "github.com/Jersyfi/hubtask/core/port/environment"
)

// A person is told of a changed way in exactly where mail goes out (UC-ID-04 check 9, UC-ID-05
// check 6): without a mail server the change is only audited.
func TestWayInNoticesAreWiredExactlyWhereMailIsConfigured(t *testing.T) {
	withMail := envport.Config{Mail: envport.MailConfig{Host: "smtp.example"}}
	if wayInNoticesFor(withMail, nil) == nil {
		t.Error("an installation with a mail server tells nobody of a changed way in")
	}
	if notices := wayInNoticesFor(envport.Config{}, nil); notices != nil {
		t.Errorf("an installation without a mail server queues notices: %#v", notices)
	}
}
