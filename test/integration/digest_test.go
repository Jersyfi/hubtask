// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"encoding/hex"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/suggestion"
)

// `digest_of` in the database and `suggestion.Digest` in the domain are the same function written
// twice, which is a thing this project otherwise refuses to have (J-10, migration 0076). It exists
// because the alternative is reading every entry's text across the wire to fingerprint it and throw
// almost all of it away — the embedding pass asks "whose text has moved" over a whole workspace.
//
// This test is the countermeasure. Two implementations of one rule is how a staleness check comes
// to pass for a suggestion made from something else, and a comment saying "they agree" is not
// evidence. The cases below are the ones a hash written twice actually gets wrong: the separator,
// empty parts, the order, and anything that is not ASCII.
func TestTheDatabaseAndTheDomainFingerprintTextTheSameWay(t *testing.T) {
	ctx := context.Background()
	pool := adminPool(ctx, t)

	for _, testCase := range []struct {
		name  string
		parts []string
	}{
		{"the ordinary case", []string{"Buy milk", "two litres"}},
		{"no notes", []string{"Buy milk", ""}},
		{"no title either", []string{"", ""}},
		// The separator's whole purpose: ("ab","c") and ("a","bc") must not collide.
		{"a boundary that could collide", []string{"ab", "c"}},
		{"the other side of it", []string{"a", "bc"}},
		{"order matters", []string{"two litres", "Buy milk"}},
		{"not ASCII", []string{"Käse kaufen", "für Sonntag — zwei Stück"}},
		{"a byte sequence that looks like the separator", []string{"aÿb", "c"}},
		{"an emoji", []string{"🥛", "milk"}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			var stored string
			if err := pool.QueryRow(ctx,
				`SELECT encode(digest_of($1, $2), 'hex')`,
				testCase.parts[0], testCase.parts[1],
			).Scan(&stored); err != nil {
				t.Fatalf("asking the database: %v", err)
			}

			computed := hex.EncodeToString(suggestion.Digest(testCase.parts...))
			if stored != computed {
				t.Errorf("the database says %s and the domain says %s - a staleness check "+
					"between the two would pass for text it was not made from", stored, computed)
			}
		})
	}
}

// And the pair that must *not* agree, because a fingerprint that collided would make a stale
// suggestion look fresh.
func TestDifferentTextFingerprintsDifferently(t *testing.T) {
	ctx := context.Background()
	pool := adminPool(ctx, t)

	var first, second string
	if err := pool.QueryRow(ctx, `SELECT encode(digest_of($1, $2), 'hex')`, "ab", "c").
		Scan(&first); err != nil {
		t.Fatalf("asking the database: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT encode(digest_of($1, $2), 'hex')`, "a", "bc").
		Scan(&second); err != nil {
		t.Fatalf("asking the database: %v", err)
	}
	if first == second {
		t.Error(`("ab","c") and ("a","bc") fingerprint the same, so the separator is doing nothing`)
	}
}
