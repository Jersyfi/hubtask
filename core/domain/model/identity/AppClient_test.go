// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// The built-in client's whole redirect rule (identity.md §15.3, ADR-0096): a loopback address by IP
// literal on any port on the desktop, Android's own scheme on Android, and nothing else.
func TestTheAppClientAcceptsOnlyItsRedirects(t *testing.T) {
	cases := []struct {
		platform AppPlatform
		uri      string
		want     bool
	}{
		{AppWindows, "http://127.0.0.1:51734/oauth", true},
		{AppLinux, "http://127.0.0.1:1/oauth", true},
		{AppLinux, "http://[::1]:65535/oauth", true},
		{AppWindows, "http://127.0.0.1/oauth", true},
		{AppWindows, "http://localhost:51734/oauth", false},
		{AppWindows, "https://127.0.0.1:51734/oauth", false},
		{AppWindows, "http://127.0.0.2:51734/oauth", false},
		{AppWindows, "http://127.0.0.1:51734/oauth/", false},
		{AppWindows, "http://127.0.0.1:51734/callback", false},
		{AppWindows, "http://127.0.0.1:51734/oauth?next=x", false},
		{AppWindows, "http://127.0.0.1:51734/oauth#x", false},
		{AppWindows, "http://user@127.0.0.1:51734/oauth", false},
		{AppWindows, "http://127.0.0.1:70000/oauth", false},
		{AppWindows, "http://127.0.0.1.evil.test/oauth", false},
		{AppWindows, AppRedirectAndroid, false},
		{AppAndroid, AppRedirectAndroid, true},
		{AppAndroid, "http://127.0.0.1:51734/oauth", false},
		{AppAndroid, "eu.hubtask.app:/oauth?x=1", false},
		{AppAndroid, "eu.hubtask.app://oauth", false},
		{AppPlatform("macos"), "http://127.0.0.1:51734/oauth", false},
	}
	for _, each := range cases {
		if got := each.platform.AllowsAppRedirect(each.uri); got != each.want {
			t.Errorf("%s %q: %v, want %v", each.platform, each.uri, got, each.want)
		}
	}
}

func appInput() NewAppClientInput {
	return NewAppClientInput{
		ID: shared.ID("0192f000-0000-7000-8000-0000000000a1"), TenantID: sessionTenant,
		Platform: "windows", Device: "  LENA-LAPTOP ", RedirectURI: "http://127.0.0.1:51734/oauth",
		Now: time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC),
	}
}

func TestAnAppClientIsPublicAndNamedByTheDevice(t *testing.T) {
	client, err := NewAppClient(appInput())
	if err != nil {
		t.Fatalf("building: %v", err)
	}
	if client.Name != "LENA-LAPTOP" || client.Confidential || !client.IsApp() ||
		client.AppPlatform != AppWindows {
		t.Errorf("client %+v", client)
	}
	if !client.AllowsRedirect("http://127.0.0.1:51734/oauth") || client.AllowsRedirect("http://127.0.0.1:51735/oauth") {
		t.Error("the row holds exactly the consent's redirect")
	}
}

func TestAnAppClientRefusesWhatItCannotBe(t *testing.T) {
	cases := map[string]struct {
		change func(*NewAppClientInput)
		code   string
	}{
		"no platform":      {func(in *NewAppClientInput) { in.Platform = "" }, "oauth.app_platform_invalid"},
		"macos":            {func(in *NewAppClientInput) { in.Platform = "macos" }, "oauth.app_platform_invalid"},
		"no device":        {func(in *NewAppClientInput) { in.Device = " " }, "oauth.app_device_required"},
		"a long device":    {func(in *NewAppClientInput) { in.Device = strings.Repeat("x", 121) }, "oauth.app_device_too_long"},
		"a foreign target": {func(in *NewAppClientInput) { in.RedirectURI = "https://evil.test/oauth" }, "oauth.redirect_uri_mismatch"},
		"android loopback": {func(in *NewAppClientInput) { in.Platform = "android" }, "oauth.redirect_uri_mismatch"},
	}
	for name, each := range cases {
		in := appInput()
		each.change(&in)
		_, err := NewAppClient(in)
		var failure *shared.Error
		if !errors.As(err, &failure) || failure.DetailCode != each.code {
			t.Errorf("%s: %v, want %s", name, err, each.code)
		}
	}
}

func TestTheAppClientIdentifierParses(t *testing.T) {
	if _, err := shared.ParseID(AppClientID.String()); err != nil {
		t.Fatalf("the built-in identifier is not one: %v", err)
	}
}
