// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/Jersyfi/hubtask/infrastructure/i18n"
)

// The exit codes. Three, and no more: a script asks whether the command worked, and if not,
// whether it was the invocation or the answer. Anything finer would be a contract this CLI has
// to keep for ever.
const (
	exitOK    = 0
	exitError = 1
	exitUsage = 2
)

// defaultTimeout bounds every call the CLI makes (CLAUDE.md rule 7). Generous, because a create
// against a cold installation is slower than a get, and still short enough that a wrong address
// fails rather than hangs.
const defaultTimeout = 30 * time.Second

// printf writes to one of the CLI's streams.
//
// The write error is dropped on purpose, once here rather than at every call site: a CLI whose
// standard output is a closed pipe has nowhere left to report that to, and `hubctl item ls | head`
// closing the pipe early is a normal thing for a shell to do rather than a failure of the command.
func printf(w io.Writer, format string, args ...any) {
	_, _ = fmt.Fprintf(w, format, args...)
}

// Streams are the three files a command may touch. Passed rather than reached for, so that the
// whole CLI is testable without a terminal.
type Streams struct {
	In  io.Reader
	Out io.Writer
	Err io.Writer
}

// CLI is one invocation: the streams, the resolved profile, and the settings the global flags
// carry. Every command is handed one.
type CLI struct {
	Streams
	Env         func(string) string
	ProfilePath string
	Profile     Profile
	Catalogue   i18n.Catalogue

	// JSON switches the output from a table for a person to the API's own payload for a pipe.
	JSON bool
	// Timeout bounds each command's call.
	Timeout time.Duration
}

// command is one verb. It parses its own flags, because a command that declared them somewhere
// else would have them declared twice - once for parsing and once for the help.
type command struct {
	name    string
	usage   string
	summary string
	run     func(ctx context.Context, cli *CLI, args []string) error
}

// group is a noun with verbs under it: `hubctl container ls`.
type group struct {
	name     string
	summary  string
	commands []command
}

// usageError is a mistake in the invocation rather than in the world. It exits 2; every other
// error exits 1.
type usageError struct {
	error
	// global says whether the command tree helps here. A mistyped flag has already had its own
	// command's usage printed by the flag set, and repeating the whole tree under it would bury
	// the one line that matters.
	global bool
}

func usagef(format string, args ...any) error {
	return usageError{error: fmt.Errorf(format, args...), global: true}
}

// errHelpRequested is `-h` on a command. Asking for help is not a mistake, so it exits 0 and
// prints nothing further - the flag set has already printed the usage.
var errHelpRequested = errors.New("help requested")

// groups is the command tree. One entry per noun, each in its own file.
func groups() []group {
	return []group{}
}

// Run is main without the process. It returns the exit code rather than calling os.Exit, which is
// what makes the whole surface - flags, help, exit codes, error text - reachable from a test.
func Run(ctx context.Context, args []string, streams Streams, env func(string) string) int {
	cli := &CLI{Streams: streams, Env: env, Timeout: defaultTimeout}

	flags := flag.NewFlagSet("hubctl", flag.ContinueOnError)
	flags.SetOutput(streams.Err)
	flags.Usage = func() { writeUsage(streams.Err) }
	var flagURL string
	flags.BoolVar(&cli.JSON, "json", false, "print the API's own payload instead of a table")
	flags.StringVar(&flagURL, "url", "", "the installation to talk to, overriding the profile")
	flags.DurationVar(&cli.Timeout, "timeout", defaultTimeout, "how long one call may take")
	showVersion := flags.Bool("version", false, "print the version and exit")

	if err := flags.Parse(args); err != nil {
		// flag.ContinueOnError has already reported the mistake, and -h is not one.
		if errors.Is(err, flag.ErrHelp) {
			writeUsage(streams.Out)
			return exitOK
		}
		return exitUsage
	}

	if *showVersion {
		printf(streams.Out, "hubctl %s (%s, built %s)\n", version, commit, buildDate)
		return exitOK
	}

	rest := flags.Args()
	if len(rest) == 0 || rest[0] == "help" {
		writeUsage(streams.Out)
		return exitOK
	}

	if err := prepare(cli, flagURL); err != nil {
		return report(streams.Err, err)
	}
	if err := dispatch(ctx, cli, rest); err != nil {
		return report(streams.Err, err)
	}
	return exitOK
}

// prepare resolves everything a command may need before knowing which command it is. The
// catalogue and the profile are cheap and always needed; the client is built by the commands that
// make a call, because `hubctl auth logout` should not need an address.
func prepare(cli *CLI, flagURL string) error {
	catalogue, err := i18n.LoadEnglish()
	if err != nil {
		return err
	}
	cli.Catalogue = catalogue

	path, err := ProfilePath(cli.Env)
	if err != nil {
		return err
	}
	cli.ProfilePath = path

	profile, err := ResolveProfile(cli.Env, path, flagURL)
	if err != nil {
		return err
	}
	cli.Profile = profile
	return nil
}

func dispatch(ctx context.Context, cli *CLI, args []string) error {
	name := args[0]
	for _, g := range groups() {
		if g.name != name {
			continue
		}
		if len(args) == 1 {
			return usagef("%s needs a command: %s", g.name, strings.Join(verbs(g), ", "))
		}
		for _, c := range g.commands {
			if c.name != args[1] {
				continue
			}
			ctx, cancel := context.WithTimeout(ctx, cli.Timeout)
			defer cancel()
			return c.run(ctx, cli, args[2:])
		}
		return usagef("%s has no command %q: %s", g.name, args[1], strings.Join(verbs(g), ", "))
	}
	return usagef("no such command: %s", name)
}

func verbs(g group) []string {
	names := make([]string, 0, len(g.commands))
	for _, c := range g.commands {
		names = append(names, c.name)
	}
	sort.Strings(names)
	return names
}

// report prints an error the way a command line tool does: the program name, a colon, one
// sentence, no stack and no JSON.
func report(w io.Writer, err error) int {
	if errors.Is(err, errHelpRequested) {
		return exitOK
	}
	var usage usageError
	if errors.As(err, &usage) {
		printf(w, "hubctl: %s\n", usage.error)
		if usage.global {
			printf(w, "\n")
			writeUsage(w)
		}
		return exitUsage
	}
	printf(w, "hubctl: %s\n", err)
	return exitError
}

func writeUsage(w io.Writer) {
	printf(w, "%s", `hubctl - the command line client for a Hubtask installation

Usage:
  hubctl [flags] <command> [arguments]

Flags:
  --url <address>    the installation to talk to, overriding the profile
  --json             print the API's own payload instead of a table
  --timeout <d>      how long one call may take (default 30s)
  --version          print the version and exit

Environment:
  HUBTASK_URL        the installation, as --url
  HUBTASK_TOKEN      the personal access token, overriding the stored one
  HUBTASK_PROFILE    where the profile is stored

Commands:
`)
	tabbed := tabwriter.NewWriter(w, 0, 0, 4, ' ', 0)
	for _, g := range groups() {
		printf(tabbed, "  %s\t%s\n", g.name, g.summary)
		for _, c := range g.commands {
			printf(tabbed, "    %s %s\t%s\n", c.name, c.usage, c.summary)
		}
	}
	printf(tabbed, "  help\tprint this text\n")
	_ = tabbed.Flush()
}
