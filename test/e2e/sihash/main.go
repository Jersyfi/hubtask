// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// A throwaway: hashes one password with the product's own Argon2 parameters so that a seeded
// account can sign in through the real route. Deleted after the walk.
package main

import (
	"fmt"
	"os"

	"github.com/Jersyfi/hubtask/core/shared/secret"
	clockadapter "github.com/Jersyfi/hubtask/infrastructure/clock"
	"github.com/Jersyfi/hubtask/infrastructure/crypto"
)

func main() {
	passwords, err := crypto.NewPasswords(clockadapter.CryptoRandom{})
	if err != nil {
		panic(err)
	}
	hash, err := passwords.Hash(secret.New(os.Args[1]))
	if err != nil {
		panic(err)
	}
	fmt.Println(hash)
}
