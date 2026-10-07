// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Command mint draws a personal access token and prints it beside the hash the database stores.
//
// It exists because the first token has to exist before the API can issue one: a script that
// starts from an empty installation writes this bootstrap credential into access_token and mints
// its working token through the API with it. The two halves - the credential and its hash - have
// to come from the same construction the server verifies with, or the session would prove that a
// token hashed one way matches a token hashed the same way and nothing else.
//
// So this is deliberately thin: it draws the entropy, builds the token through the domain's own
// constructor and hashes it through the real hasher. It talks to no database - the script that
// calls it does the insert - which keeps the credential the only thing this program knows about.
package main

import (
	"crypto/rand"
	"flag"
	"fmt"
	"os"

	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/shared/secret"
	"github.com/Jersyfi/hubtask/infrastructure/security"
)

// envSecret is the installation secret the hash is peppered with. Read from the environment
// rather than taken as an argument: an argument is visible in `ps`.
const envSecret = "HUBTASK_SECRET_KEY" //nolint:gosec // G101: the name of an environment variable.

func main() {
	tenant := flag.String("tenant", "", "the tenant the token is bound to, as a UUID")
	flag.Parse()

	if err := run(*tenant, os.Getenv(envSecret), os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "mint: %s\n", err)
		os.Exit(1)
	}
}

// run prints `<token> <hash as hex>`. One line and two fields, because the caller is a shell:
// `read -r token hash <<< "$(...)"` is the whole parsing.
func run(tenant, installationSecret string, out *os.File) error {
	if installationSecret == "" {
		return fmt.Errorf("%s is not set", envSecret)
	}
	tenantID, err := shared.ParseID(tenant)
	if err != nil {
		return fmt.Errorf("--tenant: %w", err)
	}

	material := make([]byte, identity.TokenSecretBytes)
	if _, err := rand.Read(material); err != nil {
		return fmt.Errorf("drawing the secret: %w", err)
	}
	token, err := identity.NewToken(tenantID, material)
	if err != nil {
		return fmt.Errorf("building the token: %w", err)
	}

	hash := security.NewTokenHasher(secret.New(installationSecret)).Hash(token.Secret())
	if _, err := fmt.Fprintf(out, "%s %x\n", token.Secret(), hash); err != nil {
		return err
	}
	return nil
}
