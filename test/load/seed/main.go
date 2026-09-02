// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Command seed writes the load dataset as PostgreSQL COPY text, one table per invocation.
//
// It exists because two million items cannot be created through the API in any time worth
// spending, and because a dataset that is not reproducible is not a baseline: a regression guard
// compares two runs, and if the data underneath them differs the comparison measures the data.
// So every identifier here is derived from a seed by hashing, and the same seed produces the same
// two million rows on any machine, in any order the loader chooses.
//
// It holds no database driver, deliberately. It writes to standard output and scripts/seed-load-
// dataset.sh pipes it into `psql \copy`, which keeps CLAUDE.md rule 3 - every query through the
// transaction wrapper - a statement about code that talks to a database, rather than one with an
// exception in it.
//
// The distribution is a long tail rather than an even split, because the figure H-11 records is
// throughput per vCPU *and its decay with items per tenant*: an even split would have no decay to
// measure. Rank r of n tenants gets a share proportional to 1/(r+1) - Zipf, the shape a real
// installation has, where a handful of workspaces hold most of the work and the rest hold a few
// hundred items each.
package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

// The tables this can write. One per invocation, and the caller runs it once per table with the
// same seed: identifiers are derived rather than remembered, so the four streams agree without
// anything being carried between them.
const (
	tableTenant     = "tenant"
	tableAccount    = "account"
	tableMembership = "membership"
	tableContainer  = "container"
	tableWorkItem   = "work_item"
)

// collectionsPerTenant is how many collections the items of a tenant are spread over. Three,
// because the read paths that matter - the board, the level listing, the query - are all scoped
// to one collection, and a tenant with a single collection would make every one of them a table
// scan of the whole tenant.
const collectionsPerTenant = 3

// workPackageEvery says how often an item is a child rather than a task at the root. One in five,
// which is enough that the path index and the depth filter carry a realistic mix and not so much
// that the dataset stops looking like a to-do list.
const workPackageEvery = 5

func main() {
	table := flag.String("table", "", "which table to write: tenant, account, membership, container, work_item")
	tenants := flag.Int("tenants", 200, "how many tenants the dataset holds")
	items := flag.Int("items", 2_000_000, "how many work items in total, across all tenants")
	seed := flag.String("seed", "hubtask-load", "the seed every identifier is derived from")
	flag.Parse()

	out := bufio.NewWriterSize(os.Stdout, 1<<20)
	if err := write(out, *table, *tenants, *items, *seed); err != nil {
		fmt.Fprintf(os.Stderr, "seed: %s\n", err)
		os.Exit(1)
	}
	if err := out.Flush(); err != nil {
		fmt.Fprintf(os.Stderr, "seed: %s\n", err)
		os.Exit(1)
	}
}

func write(out io.Writer, table string, tenants, items int, seed string) error {
	if tenants < 1 {
		return fmt.Errorf("--tenants must be at least 1")
	}
	if items < tenants {
		return fmt.Errorf("--items must be at least one per tenant")
	}

	shares := distribute(items, tenants)
	switch table {
	case tableTenant:
		return writeTenants(out, tenants, seed)
	case tableAccount:
		return writeAccounts(out, tenants, seed)
	case tableMembership:
		return writeMemberships(out, tenants, seed)
	case tableContainer:
		return writeContainers(out, tenants, seed)
	case tableWorkItem:
		return writeWorkItems(out, shares, seed)
	default:
		return fmt.Errorf("--table must be one of tenant, account, membership, container, work_item")
	}
}

// distribute splits the items over the tenants by rank, Zipf style, and gives the remainder to the
// largest. Every tenant gets at least one item: a tenant with none is a row that proves nothing
// and a share that rounds to zero is what a long tail does at the end.
func distribute(items, tenants int) []int {
	weights := make([]float64, tenants)
	total := 0.0
	for rank := range tenants {
		weights[rank] = 1 / float64(rank+1)
		total += weights[rank]
	}

	shares := make([]int, tenants)
	assigned := 0
	for rank := range tenants {
		share := int(float64(items) * weights[rank] / total)
		if share < 1 {
			share = 1
		}
		shares[rank] = share
		assigned += share
	}
	// The rounding lands on the biggest tenant, which is where a few hundred rows disappear
	// without changing anything about the shape.
	shares[0] += items - assigned
	if shares[0] < 1 {
		shares[0] = 1
	}
	return shares
}

// derive is the whole of the reproducibility: an identifier is a hash of what it is, never a draw.
// The version and variant bits are set so that PostgreSQL's uuid type and anything reading these
// back see an ordinary UUID rather than a value that only looks like one.
func derive(seed, kind string, index ...int) string {
	h := sha256.New()
	h.Write([]byte(seed))
	h.Write([]byte{0})
	h.Write([]byte(kind))
	for _, n := range index {
		var buf [8]byte
		binary.BigEndian.PutUint64(buf[:], uint64(n))
		h.Write(buf[:])
	}
	sum := h.Sum(nil)[:16]
	sum[6] = (sum[6] & 0x0f) | 0x40
	sum[8] = (sum[8] & 0x3f) | 0x80

	text := hex.EncodeToString(sum)
	return text[0:8] + "-" + text[8:12] + "-" + text[12:16] + "-" + text[16:20] + "-" + text[20:32]
}

// choice picks deterministically out of n possibilities, from the same derivation the identifiers
// use. There is no random source here at all - CLAUDE.md rule 4 bans one in the core, and a
// dataset generator that drew from one could not be reproduced anyway.
func choice(seed, kind string, index int, n int) int {
	sum := sha256.Sum256([]byte(derive(seed, kind, index)))
	return int(binary.BigEndian.Uint32(sum[:4]) % uint32(n))
}

// orderKey is a fixed-width, byte-ordered rank key. Fixed width because the column is compared as
// text and the database's collation is not to be trusted with the ordering of variable-length keys
// (the rank keys the application mints have the same property).
func orderKey(position int) string {
	const digits = "0123456789abcdefghijklmnopqrstuvwxyz"
	key := make([]byte, 8)
	value := position
	for i := 7; i >= 0; i-- {
		key[i] = digits[value%len(digits)]
		value /= len(digits)
	}
	return string(key)
}

// seededAt is the moment every row is stamped with. Fixed rather than time.Now(), because two runs
// of the generator have to produce the same bytes - a timestamp of "now" would make every dataset
// unique and every baseline incomparable.
var seededAt = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

func stamp(offsetDays int) string {
	return seededAt.AddDate(0, 0, -offsetDays).Format("2006-01-02 15:04:05Z07:00")
}

// row writes one COPY text line. The fields are written as they are: everything this generates is
// hexadecimal, an enum, a small integer or a title it composes itself, so none of it can contain
// a tab, a newline or a backslash. A generator that took input from outside would need the escape.
func row(out io.Writer, fields ...string) error {
	_, err := io.WriteString(out, strings.Join(fields, "\t")+"\n")
	return err
}

func tenantID(seed string, rank int) string  { return derive(seed, "tenant", rank) }
func accountID(seed string, rank int) string { return derive(seed, "account", rank) }

func writeTenants(out io.Writer, tenants int, seed string) error {
	for rank := range tenants {
		if err := row(out,
			tenantID(seed, rank),
			fmt.Sprintf("load-%04d", rank),
			fmt.Sprintf("Load tenant %04d", rank),
		); err != nil {
			return err
		}
	}
	return nil
}

func writeAccounts(out io.Writer, tenants int, seed string) error {
	for rank := range tenants {
		if err := row(out,
			accountID(seed, rank),
			tenantID(seed, rank),
			"USER",
			fmt.Sprintf("Load account %04d", rank),
			"ACTIVE",
		); err != nil {
			return err
		}
	}
	return nil
}

func writeMemberships(out io.Writer, tenants int, seed string) error {
	for rank := range tenants {
		if err := row(out,
			derive(seed, "membership", rank),
			tenantID(seed, rank),
			accountID(seed, rank),
			"TENANT",
			"OWNER",
		); err != nil {
			return err
		}
	}
	return nil
}

// writeContainers gives every tenant one hub and its collections under it. The hub is what the
// collections hang from; the items reference their collection directly, which is why nothing
// deeper is needed here.
func writeContainers(out io.Writer, tenants int, seed string) error {
	for rank := range tenants {
		hub := derive(seed, "hub", rank)
		if err := row(out,
			hub, tenantID(seed, rank), "HUB", `\N`,
			fmt.Sprintf("Hub %04d", rank), orderKey(0), accountID(seed, rank),
		); err != nil {
			return err
		}
		for n := range collectionsPerTenant {
			if err := row(out,
				derive(seed, "collection", rank, n), tenantID(seed, rank), "COLLECTION", hub,
				fmt.Sprintf("Collection %04d-%d", rank, n), orderKey(n+1), accountID(seed, rank),
			); err != nil {
				return err
			}
		}
	}
	return nil
}

// writeWorkItems is the two million. It streams: nothing here holds a row it has already written,
// so the memory this needs is the buffer and not the dataset.
func writeWorkItems(out io.Writer, shares []int, seed string) error {
	for rank, share := range shares {
		tenant := tenantID(seed, rank)
		author := accountID(seed, rank)
		// lastTask is what a work package hangs under. Kept per collection, because a child in
		// one collection under a parent in another would be a tree the application cannot make.
		lastTask := make([]string, collectionsPerTenant)
		lastPath := make([]string, collectionsPerTenant)

		for n := range share {
			collection := n % collectionsPerTenant
			id := derive(seed, "item", rank, n)
			kind, parent, path, depth := "TASK", `\N`, "/"+id+"/", "0"
			if n%workPackageEvery == workPackageEvery-1 && lastTask[collection] != "" {
				kind, parent, path, depth = "WORK_PACKAGE", lastTask[collection], lastPath[collection]+id+"/", "1"
			}

			// A third of the items are done and a third carry a due date, spread over the year
			// before the dataset's own moment - which is what gives the due and completion
			// indexes something to be selective about.
			completedAt, completed := `\N`, "false"
			if choice(seed, "completed", n, 3) == 0 {
				completedAt, completed = stamp(choice(seed, "completed-when", n, 365)), "true"
			}
			dueAt := `\N`
			if choice(seed, "due", n, 3) == 0 {
				dueAt = stamp(choice(seed, "due-when", n, 365) - 180)
			}

			if err := row(out,
				id, tenant, derive(seed, "collection", rank, collection), kind, parent, path, depth,
				fmt.Sprintf("Load item %04d-%07d", rank, n),
				completed, completedAt,
				orderKey(n), dueAt, author, stamp(choice(seed, "created", n, 365)),
			); err != nil {
				return err
			}
			if kind == "TASK" {
				lastTask[collection], lastPath[collection] = id, path
			}
		}
	}
	return nil
}
