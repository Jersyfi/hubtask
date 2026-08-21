// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Command hubctl is the command line client for a Hubtask installation.
//
// It is the dogfooding client (roadmap.md phase 5): the first consumer of the API that is not a
// test, and therefore the first one to notice when the contract is awkward. Two consequences run
// through the whole binary:
//
//   - It speaks the published contract and nothing else. The types come from openapi.yaml through
//     `make generate` (ADR-0004), so a field this client reads is a field the specification
//     promises.
//   - It renders message codes into sentences from locales/en.json rather than printing the
//     problem document it received. A client that showed raw JSON would be the counter-argument
//     to ADR-0011 rather than its first proof.
//
// The CLI's own diagnostics - a mistyped flag, a profile that is not there, a host that refuses
// the connection - are plain English here rather than message codes. They are the client's own
// text, like its help output, and never travel to another client.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
)

// Set at build time via ldflags, as for the server.
var (
	version   = "dev"
	commit    = "unknown"
	buildDate = "unknown"
)

func main() {
	// Ctrl-C cancels the request in flight rather than killing the process mid-write: every
	// command takes this context, and every call made with it stops when it is cancelled.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	os.Exit(Run(ctx, os.Args[1:], Streams{In: os.Stdin, Out: os.Stdout, Err: os.Stderr}, os.Getenv))
}
