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
}

// storedProfile is the on-disk shape.
//
// It exists so that writing the token is a deliberate act: secret.Secret masks itself in every
// serialiser (threat T-18), which is exactly right everywhere except in the one place whose whole
// job is to persist it. Reveal is called here, once, in a file a reviewer can hold in their head.
type storedProfile struct {
	BaseURL string `json:"base_url"`
	Token   string `json:"token"`
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
	return Profile{BaseURL: stored.BaseURL, Token: secret.New(stored.Token)}, nil
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

	raw, err := json.MarshalIndent(storedProfile{
		BaseURL: profile.BaseURL,
		Token:   profile.Token.Reveal(),
	}, "", "  ")
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

	if fromEnv := env(envToken); fromEnv != "" {
		profile.Token = secret.New(fromEnv)
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
