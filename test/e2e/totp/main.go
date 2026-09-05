// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Command totp prints the code an authenticator would show for a secret, right now.
//
// It exists for the same reason `mint` next door does: the end-to-end session has to be somebody
// with a phone, and it has no phone. What it does have is the base32 secret the enrolment answered
// once, which is exactly what an authenticator is given - so this program is the authenticator,
// and nothing more than that.
//
// It goes through the domain's own TotpCode rather than reimplementing RFC 6238, for `mint`'s
// reason: a code computed here the way this program computes codes would prove that two identical
// implementations agree, which is not the question. The question is whether the server accepts
// what an authenticator produces, and the only way to ask it is to produce one the same way the
// verifier expects.
//
// The secret arrives in the environment rather than as an argument: an argument is visible in `ps`,
// and this one is the whole second factor.
package main

import (
	"encoding/base32"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/identity"
)

// envTotpSecret is the base32 secret, as `hubctl mfa enroll` printed it.
const envTotpSecret = "HUBTASK_TOTP_SECRET" //nolint:gosec // G101: the name of an environment variable

func main() {
	code, err := run(os.Getenv(envTotpSecret), time.Now())
	if err != nil {
		fmt.Fprintf(os.Stderr, "totp: %s\n", err)
		os.Exit(1)
	}
	fmt.Println(code)
}

func run(base32Secret string, at time.Time) (string, error) {
	if base32Secret == "" {
		return "", fmt.Errorf("%s is not set", envTotpSecret)
	}
	// Unpadded upper case, which is how every authenticator is given one and how
	// identity.TotpSecretBase32 writes it. The spacing people introduce reading a secret off a
	// screen is stripped, because a script pasting one should not fail on a newline.
	cleaned := strings.ToUpper(strings.NewReplacer(" ", "", "\t", "", "\n", "", "=", "").
		Replace(base32Secret))
	secret, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(cleaned)
	if err != nil {
		return "", fmt.Errorf("%s is not base32: %w", envTotpSecret, err)
	}
	return identity.TotpCode(secret, identity.TotpStep(at)), nil
}
