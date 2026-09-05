// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// The environment. Named like the server's variables, because a person who has configured one
// installation should not have to learn a second vocabulary to talk to it.
const (
	envURL     = "HUBTASK_URL"
	envProfile = "HUBTASK_PROFILE"
	// The linter sees `token` next to a string literal and suspects a credential. This is the
	// name of the variable that carries one, which is the opposite: it is meant to be published.
	envToken = "HUBTASK_TOKEN" //nolint:gosec // G101: the name of an environment variable, not a secret.
	// The password a sign-in and a step-up ask for. It is read from here or from a pipe, never
	// from an argument, for the reason there is no --token flag: an argument is visible in `ps`
	// and lands in the shell history.
	envPassword = "HUBTASK_PASSWORD"
	// The authenticator's current code, for the same reason - and because a code is worth six
	// digits for thirty seconds, which is exactly as long as a shell history is unhelpful.
	envTotp = "HUBTASK_TOTP"
	// The workspace, in multi mode. A sign-in has no credential to read the tenant off yet
	// (multi-tenancy.md §3), so it has to be said.
	envTenant = "HUBTASK_TENANT"
)

// profileFileMode and profileDirMode keep the credential to its owner. A token is a password;
// a world-readable file holding one is the same mistake as a world-readable ~/.ssh.
const (
	profileFileMode os.FileMode = 0o600
	profileDirMode  os.FileMode = 0o700
)

// Profile is what hubctl needs in order to talk to an installation: where it is, and who is
// calling. One profile rather than a set of named contexts - a second installation is a real
// second use case and can have the abstraction then.
type Profile struct {
	// BaseURL is the installation, not the API path. hubctl appends /api/v1 itself, so that the
	// version of the interface it speaks is a property of the binary rather than of something a
	// user typed once and forgot.
	BaseURL string
	Token   secret.Secret
	// Tenant is the workspace this shell talks to, in an installation running more than one. It
	// is §3's weakest source of resolution - it confirms, never overrules - and it is here rather
	// than on every command because a person works in one workspace at a time.
	Tenant string
	// Session is the sign-in `hubctl login` opened, when there is one. A personal access token
	// and a session are two credentials, and holding both would leave which one a command used
	// to the order of two fields; so the profile holds one at a time and each sign-in replaces
	// the other.
	Session Session
}

// Session is the pair a sign-in answered, held between invocations.
//
// The access token is what travels on every call, and it lives fifteen minutes - far less than
// the time between two commands somebody types. So the refresh token is held beside it and
// exchanged when the access token is nearly out (H-01): without that, `hubctl login` would be
// good for a quarter of an hour and a scripted session would sign in over and over.
type Session struct {
	// ID is the session row, so that `hubctl session ls` can say which line is this shell.
	ID string
	// AccessToken is the bearer credential. AccessExpiresAt is when it stops verifying.
	AccessToken     secret.Secret
	AccessExpiresAt time.Time
	// RefreshToken buys the next pair, exactly once: the exchange retires it, and presenting a
	// retired one invalidates the whole family. That is why a rotation is written to the profile
	// before the new pair is used for anything.
	RefreshToken     secret.Secret
	RefreshExpiresAt time.Time
}

// IsEmpty reports whether there is a session at all.
func (s Session) IsEmpty() bool { return s.AccessToken.IsEmpty() }

// Credential is what travels on a call: the session's access token where a sign-in opened one,
// otherwise the personal access token.
//
// The choice is made here rather than by writing one over the other, because the two fields are
// also what gets persisted: a resolution that overwrote Token would save the session's access
// token as though somebody had pasted it, and a profile would then hold a fifteen-minute
// credential in the field meant for a thirty-day one.
func (p Profile) Credential() secret.Secret {
	if !p.Session.IsEmpty() {
		return p.Session.AccessToken
	}
	return p.Token
}

// refreshDue reports whether the access token is out, or close enough to it that the next call
// would be the one to find out. The margin is what keeps a command from failing on a credential
// that was valid when the profile was read and is not when the call arrives.
func (s Session) refreshDue(now time.Time) bool {
	return !s.RefreshToken.IsEmpty() && !now.Before(s.AccessExpiresAt.Add(-refreshMargin))
}

// refreshMargin is how early a session is renewed. Generous next to fifteen minutes, and larger
// than any call this client makes.
const refreshMargin = time.Minute

// storedProfile is the on-disk shape.
//
// It exists so that writing the token is a deliberate act: secret.Secret masks itself in every
// serialiser (threat T-18), which is exactly right everywhere except in the one place whose whole
// job is to persist it. Reveal is called here, once, in a file a reviewer can hold in their head.
type storedProfile struct {
	BaseURL string         `json:"base_url"`
	Token   string         `json:"token"`
	Tenant  string         `json:"tenant,omitempty"`
	Session *storedSession `json:"session,omitempty"`
}

type storedSession struct {
	ID               string    `json:"id"`
	AccessToken      string    `json:"access_token"`
	AccessExpiresAt  time.Time `json:"access_token_expires_at"`
	RefreshToken     string    `json:"refresh_token"`
	RefreshExpiresAt time.Time `json:"refresh_token_expires_at"`
}

// ProfilePath is where the profile lives: $HUBTASK_PROFILE if set, otherwise the platform's
// configuration directory. os.UserConfigDir gives ~/.config on Linux, ~/Library/Application
// Support on macOS and %AppData% on Windows - the CLI runs natively on all three
// (support-matrix.md §4), so the location is the platform's answer rather than ours.
func ProfilePath(env func(string) string) (string, error) {
	if explicit := env(envProfile); explicit != "" {
		return explicit, nil
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("no configuration directory on this system, and %s is not set: %w", envProfile, err)
	}
	return filepath.Join(dir, "hubtask", "profile.json"), nil
}

// LoadProfile reads the stored profile. A file that is not there is not an error: it is somebody
// who has not signed in yet, and the caller decides whether that matters.
func LoadProfile(path string) (Profile, error) {
	//nolint:gosec // G304: the profile is the file the user named; reading it is the point.
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Profile{}, nil
	}
	if err != nil {
		return Profile{}, fmt.Errorf("reading %s: %w", path, err)
	}

	var stored storedProfile
	if err := json.Unmarshal(raw, &stored); err != nil {
		return Profile{}, fmt.Errorf("%s is not a profile hubctl wrote: %w", path, err)
	}
	profile := Profile{
		BaseURL: stored.BaseURL,
		Token:   secret.New(stored.Token),
		Tenant:  stored.Tenant,
	}
	if stored.Session != nil {
		profile.Session = Session{
			ID:               stored.Session.ID,
			AccessToken:      secret.New(stored.Session.AccessToken),
			AccessExpiresAt:  stored.Session.AccessExpiresAt,
			RefreshToken:     secret.New(stored.Session.RefreshToken),
			RefreshExpiresAt: stored.Session.RefreshExpiresAt,
		}
	}
	return profile, nil
}

// SaveProfile writes the profile with the credential in it.
//
// Written to a temporary file and renamed, so that an interrupted write leaves the old profile
// rather than half a new one. The temporary file is created with the same mode as the target: a
// file that is briefly world-readable is world-readable for as long as somebody is looking.
func SaveProfile(path string, profile Profile) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, profileDirMode); err != nil {
		return fmt.Errorf("creating %s: %w", dir, err)
	}

	stored := storedProfile{
		BaseURL: profile.BaseURL,
		Token:   profile.Token.Reveal(),
		Tenant:  profile.Tenant,
	}
	if !profile.Session.IsEmpty() {
		stored.Session = &storedSession{
			ID:               profile.Session.ID,
			AccessToken:      profile.Session.AccessToken.Reveal(),
			AccessExpiresAt:  profile.Session.AccessExpiresAt,
			RefreshToken:     profile.Session.RefreshToken.Reveal(),
			RefreshExpiresAt: profile.Session.RefreshExpiresAt,
		}
	}
	raw, err := json.MarshalIndent(stored, "", "  ")
	if err != nil {
		return fmt.Errorf("writing the profile: %w", err)
	}

	temporary, err := os.CreateTemp(dir, ".profile-*.json")
	if err != nil {
		return fmt.Errorf("creating a temporary file in %s: %w", dir, err)
	}
	defer func() { _ = os.Remove(temporary.Name()) }()

	if err := temporary.Chmod(profileFileMode); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("restricting %s: %w", temporary.Name(), err)
	}
	if _, err := temporary.Write(append(raw, '\n')); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("writing %s: %w", temporary.Name(), err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("closing %s: %w", temporary.Name(), err)
	}
	if err := os.Rename(temporary.Name(), path); err != nil {
		return fmt.Errorf("replacing %s: %w", path, err)
	}
	return nil
}

// ForgetProfile removes the stored profile. Removing a file that is not there is what was asked
// for, not a failure.
func ForgetProfile(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("removing %s: %w", path, err)
	}
	return nil
}

// ResolveProfile decides which installation and which credential this invocation uses.
//
// The order is environment over file, and the flag over both. A CI job sets two variables and
// needs no profile on disk; a person signs in once and needs no variables. What is deliberately
// absent is a --token flag: an argument is visible in `ps` and lands in the shell history, so the
// token arrives through the environment or through the file, and hubctl auth login reads it from
// a pipe.
func ResolveProfile(env func(string) string, path, flagURL string) (Profile, error) {
	profile, err := LoadProfile(path)
	if err != nil {
		return Profile{}, err
	}

	// The order is deliberate and it is the one the help text names: a variable in the shell
	// beats what is stored, and a stored session beats a stored personal access token - the
	// sign-in that happened last is the one that wrote the file.
	if fromEnv := env(envToken); fromEnv != "" {
		profile.Token = secret.New(fromEnv)
		profile.Session = Session{}
	}
	if fromEnv := env(envTenant); fromEnv != "" {
		profile.Tenant = fromEnv
	}
	for _, candidate := range []string{flagURL, env(envURL)} {
		if candidate != "" {
			profile.BaseURL = candidate
			break
		}
	}

	if profile.BaseURL == "" {
		return Profile{}, nil
	}
	normalised, err := NormaliseBaseURL(profile.BaseURL)
	if err != nil {
		return Profile{}, err
	}
	profile.BaseURL = normalised
	return profile, nil
}

// NormaliseBaseURL checks that what somebody typed is an installation address and strips the
// trailing slash, so that joining a path never produces a double one.
//
// It refuses anything but http and https rather than letting net/http fail later with a message
// about an unsupported protocol scheme: the answer to "hubctl auth login --url localhost:8080" is
// that the scheme is missing, not that a request failed.
func NormaliseBaseURL(raw string) (string, error) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("%q is not a URL: %w", raw, err)
	}
	switch {
	case parsed.Scheme != "http" && parsed.Scheme != "https":
		return "", fmt.Errorf("%q needs an http:// or https:// scheme", raw)
	case parsed.Host == "":
		return "", fmt.Errorf("%q names no host", raw)
	case parsed.RawQuery != "" || parsed.Fragment != "":
		return "", fmt.Errorf("%q is an address, not a request - drop the query and the fragment", raw)
	}
	parsed.Path = strings.TrimSuffix(parsed.Path, "/")
	return parsed.String(), nil
}
