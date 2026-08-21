// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// validToken has the shape identity.ParseToken demands: the prefix, a tenant as 32 hex digits,
// and 43 base64url characters of secret.
const validToken = "hbt_pat_0123456789abcdef0123456789abcdef_" +
	"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"

// invokeAgainst runs the CLI against a stub installation, with the profile in a temporary file.
func invokeAgainst(t *testing.T, stub *installation, env map[string]string, stdin string, args ...string) (int, string, string) {
	t.Helper()
	if env == nil {
		env = map[string]string{}
	}
	if _, set := env[envURL]; !set {
		env[envURL] = stub.server.URL
	}
	return invoke(t, env, stdin, args...)
}

func TestSigningInVerifiesTheCredentialBeforeStoringIt(t *testing.T) {
	stub := serve(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tenancy_mode":"single"}`))
	})
	profile := filepath.Join(t.TempDir(), "profile.json")

	code, out, errOut := invokeAgainst(t, stub, map[string]string{envProfile: profile},
		validToken+"\n", "auth", "login", "--url", stub.server.URL)
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if stub.request.URL.Path != APIPath+capabilitiesPath {
		t.Errorf("the sign-in called %s, want the operation that needs no scope", stub.request.URL.Path)
	}
	// Nothing on standard output: a sign-in has no payload, and a script piping stdout should
	// get nothing rather than a sentence to skip.
	if out != "" {
		t.Errorf("standard output carried %q", out)
	}

	stored, err := LoadProfile(profile)
	if err != nil {
		t.Fatalf("loading the stored profile: %v", err)
	}
	if stored.Token.Reveal() != validToken || stored.BaseURL != stub.server.URL {
		t.Errorf("the profile does not hold what was signed in with: %+v", stored)
	}
}

// A profile holding a credential the installation refuses is worse than no profile: every later
// command then fails with an answer about the token rather than about the sign-in.
func TestARefusedCredentialIsNotStored(t *testing.T) {
	stub := serve(t, func(w http.ResponseWriter, _ *http.Request) {
		problemJSON(w, http.StatusUnauthorized, map[string]any{
			"status": 401, "code": "unauthenticated", "detail_code": "access.token_unknown",
		})
	})
	profile := filepath.Join(t.TempDir(), "profile.json")

	code, _, errOut := invokeAgainst(t, stub, map[string]string{envProfile: profile},
		validToken, "auth", "login", "--url", stub.server.URL)
	if code != exitError {
		t.Fatalf("exit %d, want %d", code, exitError)
	}
	if strings.Contains(errOut, "{") {
		t.Errorf("the refusal was printed as a document: %q", errOut)
	}

	stored, err := LoadProfile(profile)
	if err != nil {
		t.Fatalf("loading: %v", err)
	}
	if !stored.Token.IsEmpty() {
		t.Error("a refused credential was stored anyway")
	}
}

// A truncated paste should read as a truncated paste, not as a rejected credential.
func TestATokenOfTheWrongShapeIsRefusedBeforeAnyCall(t *testing.T) {
	stub := serve(t, func(http.ResponseWriter, *http.Request) {
		t.Error("a call was made with a token that cannot be one")
	})

	code, _, errOut := invokeAgainst(t, stub, nil, "hbt_pat_short", "auth", "login", "--url", stub.server.URL)
	if code != exitError {
		t.Fatalf("exit %d, want %d", code, exitError)
	}
	if errOut == "" {
		t.Error("nothing was said about the token")
	}
}

func TestSigningInWithoutATokenSaysWhereToPutOne(t *testing.T) {
	stub := serve(t, func(http.ResponseWriter, *http.Request) {})
	code, _, errOut := invokeAgainst(t, stub, nil, "", "auth", "login", "--url", stub.server.URL)
	if code != exitError {
		t.Fatalf("exit %d, want %d", code, exitError)
	}
	if !strings.Contains(errOut, envToken) {
		t.Errorf("the message %q does not name the environment variable", errOut)
	}
}

func TestStatusNamesWhereTheCredentialComesFrom(t *testing.T) {
	profile := filepath.Join(t.TempDir(), "profile.json")
	if err := SaveProfile(profile, Profile{BaseURL: "https://stored.example.com", Token: secret.New("stored")}); err != nil {
		t.Fatalf("saving: %v", err)
	}

	for _, tc := range []struct {
		name string
		env  map[string]string
		want string
	}{
		{"from the profile", map[string]string{envProfile: profile}, sourceProfile},
		{"from the environment",
			map[string]string{envProfile: profile, envToken: validToken}, sourceEnvironment},
		{"from nowhere",
			map[string]string{envProfile: filepath.Join(t.TempDir(), "absent.json")}, sourceNone},
	} {
		t.Run(tc.name, func(t *testing.T) {
			code, out, errOut := invoke(t, tc.env, "", "--json", "auth", "status")
			if code != exitOK {
				t.Fatalf("exit %d: %s", code, errOut)
			}
			if !strings.Contains(out, `"token_source": "`+tc.want+`"`) {
				t.Errorf("status %q does not name %s", out, tc.want)
			}
		})
	}
}

func TestLoggingOutRemovesTheProfileAndWarnsAboutTheEnvironment(t *testing.T) {
	profile := filepath.Join(t.TempDir(), "profile.json")
	if err := SaveProfile(profile, Profile{BaseURL: "https://h", Token: secret.New("t")}); err != nil {
		t.Fatalf("saving: %v", err)
	}

	code, _, errOut := invoke(t, map[string]string{envProfile: profile, envToken: validToken},
		"", "auth", "logout")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}

	stored, err := LoadProfile(profile)
	if err != nil {
		t.Fatalf("loading: %v", err)
	}
	if !stored.Token.IsEmpty() {
		t.Error("the credential survived the logout")
	}
	// Otherwise the next command still works and nobody can see why.
	if !strings.Contains(errOut, envToken) {
		t.Errorf("nothing warned that %s is still set: %q", envToken, errOut)
	}
}
