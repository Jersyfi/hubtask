// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	openapitypes "github.com/oapi-codegen/runtime/types"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	clockadapter "github.com/Jersyfi/hubtask/infrastructure/clock"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// The reference client of offline-sync.md, in the CLI (N-12): `hubctl sync pull` pages the
// delta or the initial synchronisation as JSON lines with the cursor last, `hubctl sync push`
// sends a file of mutations and answers one result per line, and `hubctl sync devices` lists
// and forgets the devices. hubctl is a device like any other: it mints one identifier, keeps it
// in the profile, and stamps its readings from a hybrid clock of its own that ticks the way the
// server's does - a reference client that stamped bare timestamps would be the client §4.1
// warns about.

const (
	syncPullPath     = "/sync:pull"
	syncSnapshotPath = "/sync:snapshot"
	syncPushPath     = "/sync:push"
	syncDevicesPath  = "/sync/devices"

	// syncPushBatch is how many mutations one push carries, the contract's maximum.
	syncPushBatch = 500
	// syncLineLimit bounds one line of a mutations file, the stream's own bound.
	syncLineLimit = 1 << 20
	// syncPlatform is what hubctl says it is in the device list.
	syncPlatform = "hubctl"
)

func syncGroup() group {
	return group{
		name:    "sync",
		summary: "the offline synchronisation, as a device",
		commands: []command{
			{
				name:    "pull",
				usage:   "[--cursor <cursor>] [--scope <container-id>[:SELF|SUBTREE]]... [--all] [--limit <n>] [--device <id>]",
				summary: "pull the changes since a cursor - or everything, from nothing - as JSON lines, the cursor last",
				run:     syncPull,
			},
			{
				name:    "snapshot",
				usage:   "[--out <file>] [--scope <container-id>[:SELF|SUBTREE]]... [--device <id>] [--apply] [--wait <d>]",
				summary: "the initial synchronisation as one stream, written to a file or standard output; --apply keeps its cursor in the profile",
				run:     syncSnapshot,
				waits:   true,
			},
			{
				name:    "push",
				usage:   "--file <mutations.jsonl> [--device <id>] [--clock-offset <duration>]",
				summary: "push a file of mutations, one JSON line each, and print one result per line",
				run:     syncPush,
			},
			{
				name:    "devices",
				usage:   "ls | forget <id>",
				summary: "the devices that synchronise for this account",
				run:     syncDevices,
			},
		},
	}
}

// syncPull pages the changes and writes them as JSON lines: one record per line, then one line
// holding the cursor to continue from - last, so that a pipe reading the records finds it where
// a pipe would look.
func syncPull(ctx context.Context, cli *CLI, args []string) error {
	flags := commandFlags(cli, "sync", "pull",
		"[--cursor <cursor>] [--scope <container-id>[:SELF|SUBTREE]]... [--all] [--limit <n>] [--device <id>]")
	cursor := flags.String("cursor", "", "continue from this cursor; unset starts an initial synchronisation")
	fromProfile := flags.Bool("continue", false, "continue from the cursor the profile keeps - the one `sync snapshot --apply` kept")
	var scopes scopeFlags
	flags.Var(&scopes, "scope", "hold this container, to its depth (repeatable)")
	all := flags.Bool("all", false, "keep paging until there is no more")
	limit := flags.Int("limit", 0, "how many records per page (the server decides when unset)")
	deviceFlag := flags.String("device", "", "act as this device; unset uses the one kept in the profile")
	if err := parseCommand(flags, args); err != nil {
		return err
	}
	if flags.NArg() > 0 {
		return usagef("unexpected argument %q: hubctl sync pull takes only flags", flags.Arg(0))
	}
	if *fromProfile {
		if *cursor != "" {
			return usagef("--continue and --cursor name two cursors: hubctl sync pull takes one")
		}
		if cli.Profile.Cursor == "" {
			return usagef("the profile keeps no cursor yet: hubctl sync snapshot --apply keeps one")
		}
		*cursor = cli.Profile.Cursor
	}
	device, err := cli.syncDevice(*deviceFlag)
	if err != nil {
		return err
	}
	client, err := cli.client()
	if err != nil {
		return err
	}

	// The request as the contract spells it, built by hand rather than through the generated
	// type: the scopes are anonymous structs there, and a client's request is its own to spell.
	request := map[string]any{"device_id": device.String(), "platform": syncPlatform}
	if *cursor != "" {
		request["cursor"] = *cursor
	}
	if *limit > 0 {
		request["limit"] = *limit
	}
	if len(scopes) > 0 {
		request["scopes"] = scopes.request()
	}

	out := bufio.NewWriter(cli.Out)
	defer func() { _ = out.Flush() }()
	encoder := json.NewEncoder(out)
	for {
		var page openapi.SyncPullResponse
		if err := client.Post(ctx, syncPullPath, request, &page); err != nil {
			return err
		}
		for _, change := range page.Changes {
			if err := encoder.Encode(change); err != nil {
				return err
			}
		}
		if !page.HasMore || !*all {
			if *fromProfile {
				// Continuing from the profile's cursor moves it: the next --continue starts
				// where this one ended.
				cli.Profile.Cursor = page.Cursor
				if err := SaveProfile(cli.ProfilePath, cli.Profile); err != nil {
					return err
				}
			}
			return encoder.Encode(map[string]any{"cursor": page.Cursor, "has_more": page.HasMore})
		}
		request["cursor"] = page.Cursor
	}
}

// syncSnapshot takes the initial synchronisation as one stream (SY-C, P-12) and writes it where
// it is told - a file, or standard output - as it arrives, line for line. The stream ends in a
// cursor line; `--apply` keeps that cursor in the profile, which is hubctl's store, so that
// `hubctl sync pull --continue` takes the delta from where the snapshot left the device. A stream
// that ends without the cursor line was cut short and is reported as such: nothing is kept, and
// the device starts again.
func syncSnapshot(ctx context.Context, cli *CLI, args []string) error {
	flags := commandFlags(cli, "sync", "snapshot",
		"[--out <file>] [--scope <container-id>[:SELF|SUBTREE]]... [--device <id>] [--apply] [--wait <d>]")
	out := flags.String("out", "", "write the stream here; unset writes it to standard output")
	var scopes scopeFlags
	flags.Var(&scopes, "scope", "hold this container, to its depth (repeatable)")
	deviceFlag := flags.String("device", "", "act as this device; unset uses the one kept in the profile")
	apply := flags.Bool("apply", false, "keep the cursor the stream ends on in the profile, for sync pull --continue")
	wait := waitFlag(flags)
	if err := parseCommand(flags, args); err != nil {
		return err
	}
	if flags.NArg() > 0 {
		return usagef("unexpected argument %q: hubctl sync snapshot takes only flags", flags.Arg(0))
	}
	device, err := cli.syncDevice(*deviceFlag)
	if err != nil {
		return err
	}
	client, err := cli.client()
	if err != nil {
		return err
	}
	request := map[string]any{"device_id": device.String(), "platform": syncPlatform}
	if len(scopes) > 0 {
		request["scopes"] = scopes.request()
	}

	sink := cli.Out
	if *out != "" {
		file, err := os.Create(*out)
		if err != nil {
			return fmt.Errorf("opening %s: %w", *out, err)
		}
		defer func() { _ = file.Close() }()
		sink = file
	}

	bounded, cancel := context.WithTimeout(ctx, *wait)
	defer cancel()
	response, err := client.OpenSnapshot(bounded, request)
	if err != nil {
		return err
	}
	defer func() { _ = response.Body.Close() }()

	written := bufio.NewWriter(sink)
	defer func() { _ = written.Flush() }()
	scanner := bufio.NewScanner(response.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), syncLineLimit)
	records, cursor := 0, ""
	for scanner.Scan() {
		line := scanner.Bytes()
		if _, err := written.Write(line); err != nil {
			return err
		}
		if err := written.WriteByte('\n'); err != nil {
			return err
		}
		var trailer struct {
			Cursor string `json:"cursor"`
			Entity string `json:"entity"`
		}
		if err := json.Unmarshal(line, &trailer); err == nil && trailer.Cursor != "" && trailer.Entity == "" {
			cursor = trailer.Cursor
			continue
		}
		records++
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("reading the snapshot: %w", err)
	}
	if cursor == "" {
		return errors.New("the snapshot ended before its cursor line: it was cut short, and nothing is kept - start again")
	}
	if *apply {
		cli.Profile.Cursor = cursor
		if err := SaveProfile(cli.ProfilePath, cli.Profile); err != nil {
			return err
		}
	}
	printf(cli.Err, "hubctl: %d records, the cursor last\n", records)
	return nil
}

// syncPush reads a file of mutations - one JSON object per line, the contract's SyncMutation -
// stamps a reading on any that carries none, and pushes them in the order they were written. The
// answer is one result per line, in the same order. A line is sent as it was read: a client's
// queue is its own, and hubctl adds only what a device would - the reading.
func syncPush(ctx context.Context, cli *CLI, args []string) error {
	flags := commandFlags(cli, "sync", "push", "--file <mutations.jsonl> [--device <id>] [--clock-offset <duration>]")
	file := flags.String("file", "", "the mutations, one JSON line each; - reads standard input")
	deviceFlag := flags.String("device", "", "act as this device; unset uses the one kept in the profile")
	offset := flags.Duration("clock-offset", 0, "skew the readings hubctl stamps by this much, to walk the clock bounding")
	if err := parseCommand(flags, args); err != nil {
		return err
	}
	if *file == "" {
		return usagef("say which file holds the mutations: --file <mutations.jsonl>")
	}
	device, err := cli.syncDevice(*deviceFlag)
	if err != nil {
		return err
	}
	mutations, err := readMutations(cli, *file)
	if err != nil {
		return err
	}
	clock := &deviceClock{cli: cli, device: device.String(), offset: *offset}
	for i := range mutations {
		if err := clock.stamp(mutations[i]); err != nil {
			return err
		}
	}
	if err := clock.save(); err != nil {
		return err
	}

	client, err := cli.client()
	if err != nil {
		return err
	}
	out := bufio.NewWriter(cli.Out)
	defer func() { _ = out.Flush() }()
	encoder := json.NewEncoder(out)
	for start := 0; start < len(mutations); start += syncPushBatch {
		end := min(start+syncPushBatch, len(mutations))
		request := map[string]any{
			"device_id": device.String(), "platform": syncPlatform, "mutations": mutations[start:end],
		}
		var response openapi.SyncPushResponse
		if err := client.Post(ctx, syncPushPath, request, &response); err != nil {
			return err
		}
		for _, result := range response.Results {
			if err := encoder.Encode(result); err != nil {
				return err
			}
		}
	}
	return nil
}

// readMutations reads the file as JSON lines. Each line is kept as the object it was, so that
// what the server receives is what the file says - with the reading added where it was missing.
func readMutations(cli *CLI, path string) ([]map[string]any, error) {
	source := cli.In
	if path != "-" {
		//nolint:gosec // G304: the file is the one the user named; reading it is the point.
		handle, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		defer func() { _ = handle.Close() }()
		source = handle
	}
	scanner := bufio.NewScanner(source)
	scanner.Buffer(make([]byte, 0, 64<<10), syncLineLimit)
	var mutations []map[string]any
	for line := 1; scanner.Scan(); line++ {
		text := strings.TrimSpace(scanner.Text())
		if text == "" || strings.HasPrefix(text, "#") {
			continue
		}
		var mutation map[string]any
		if err := json.Unmarshal([]byte(text), &mutation); err != nil {
			return nil, usagef("line %d of %s is not a JSON object: %v", line, path, err)
		}
		if _, named := mutation["op_id"]; !named {
			return nil, usagef("line %d of %s names no op_id: a mutation is idempotent by it, and hubctl does not mint one", line, path)
		}
		mutations = append(mutations, mutation)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if len(mutations) == 0 {
		return nil, usagef("%s holds no mutation", path)
	}
	return mutations, nil
}

// deviceClock is hubctl's hybrid logical clock: the last reading kept in the profile between
// invocations and ticked from there, so that two mutations stamped in one session never share
// a reading and a reading never goes backwards, the way the server's clock behaves (§4.1). The
// offset is the walk's: a device whose clock is hours out, from a shell.
type deviceClock struct {
	cli    *CLI
	device string
	offset time.Duration
	last   shared.HLC
	loaded bool
}

func (c *deviceClock) next() (string, error) {
	if !c.loaded {
		if raw := c.cli.Profile.Clock; raw != "" {
			if parsed, err := shared.ParseHLC(raw); err == nil {
				c.last = parsed
			}
		}
		c.loaded = true
	}
	reading, err := c.last.Tick(time.Now().Add(c.offset), c.device)
	if err != nil {
		return "", err
	}
	c.last = reading
	return reading.String(), nil
}

// stamp gives the mutation and each of its fields a reading where it carries none.
func (c *deviceClock) stamp(mutation map[string]any) error {
	fields, _ := mutation["fields"].(map[string]any)
	for name, raw := range fields {
		field, _ := raw.(map[string]any)
		if field == nil {
			continue
		}
		if stamped, _ := field["hlc"].(string); stamped == "" {
			reading, err := c.next()
			if err != nil {
				return err
			}
			field["hlc"] = reading
			fields[name] = field
		}
	}
	if stamped, _ := mutation["hlc"].(string); stamped == "" && len(fields) == 0 {
		reading, err := c.next()
		if err != nil {
			return err
		}
		mutation["hlc"] = reading
	}
	return nil
}

// save keeps the clock where the next invocation finds it.
func (c *deviceClock) save() error {
	if !c.loaded || c.last.IsZero() || c.last.String() == c.cli.Profile.Clock {
		return nil
	}
	c.cli.Profile.Clock = c.last.String()
	return SaveProfile(c.cli.ProfilePath, c.cli.Profile)
}

// syncDevice is the device the commands act as: the flag, or the one kept in the profile - minted
// on first use, so that every invocation of this shell is the same device to the server.
func (cli *CLI) syncDevice(flag string) (openapitypes.UUID, error) {
	if flag != "" {
		return cli.parseID("--device", flag)
	}
	if cli.Profile.Device == "" {
		cli.Profile.Device = clockadapter.NewUUIDv7(clockadapter.System{}).NewID().String()
		if err := SaveProfile(cli.ProfilePath, cli.Profile); err != nil {
			return openapitypes.UUID{}, err
		}
		printf(cli.Err, "hubctl: this shell synchronises as device %s from now on\n", cli.Profile.Device)
	}
	return cli.parseID("the profile's device", cli.Profile.Device)
}

// scopeFlags is `--scope <id>[:SELF|SUBTREE]`, repeatable.
type scopeFlags []scopeFlag

type scopeFlag struct {
	container string
	depth     string
}

func (s *scopeFlags) String() string {
	parts := make([]string, 0, len(*s))
	for _, scope := range *s {
		parts = append(parts, scope.container+":"+scope.depth)
	}
	return strings.Join(parts, ",")
}

func (s *scopeFlags) Set(value string) error {
	container, depth, _ := strings.Cut(value, ":")
	if _, err := shared.ParseID(container); err != nil {
		return errors.New("a scope names a container by its identifier")
	}
	if depth == "" {
		depth = "SUBTREE"
	}
	if depth != "SELF" && depth != "SUBTREE" {
		return errors.New("a scope's depth is SELF or SUBTREE")
	}
	*s = append(*s, scopeFlag{container: container, depth: depth})
	return nil
}

func (s scopeFlags) request() []map[string]any {
	out := make([]map[string]any, 0, len(s))
	for _, scope := range s {
		out = append(out, map[string]any{"container_id": scope.container, "depth": scope.depth})
	}
	return out
}

// syncDevices is `hubctl sync devices ls` and `hubctl sync devices forget <id>`.
func syncDevices(ctx context.Context, cli *CLI, args []string) error {
	if len(args) == 0 {
		return usagef("hubctl sync devices ls | forget <id>")
	}
	switch args[0] {
	case "ls":
		return syncDevicesList(ctx, cli, args[1:])
	case "forget":
		return syncDevicesForget(ctx, cli, args[1:])
	default:
		return usagef("unknown verb %q: hubctl sync devices ls | forget <id>", args[0])
	}
}

func syncDevicesList(ctx context.Context, cli *CLI, args []string) error {
	flags := commandFlags(cli, "sync", "devices ls", "")
	if err := parseCommand(flags, args); err != nil {
		return err
	}
	client, err := cli.client()
	if err != nil {
		return err
	}
	var devices []openapi.SyncDevice
	if err := client.Get(ctx, syncDevicesPath, nil, &devices); err != nil {
		return err
	}
	rows := make([][]string, 0, len(devices))
	for _, device := range devices {
		rows = append(rows, []string{
			device.Id.String(), text(device.Platform), text(device.DisplayName),
			shortTime(device.LastSeenAt), strconv.FormatBool(device.Blocked),
		})
	}
	return cli.Emit(devices, Table{Columns: []string{"id", "platform", "name", "last seen", "blocked"}, Rows: rows})
}

func syncDevicesForget(ctx context.Context, cli *CLI, args []string) error {
	flags := commandFlags(cli, "sync", "devices forget", "<id>")
	if err := parseCommand(flags, args); err != nil {
		return err
	}
	if flags.NArg() != 1 {
		return usagef("say which device to forget: hubctl sync devices forget <id>")
	}
	device, err := cli.parseID("the device", flags.Arg(0))
	if err != nil {
		return err
	}
	client, err := cli.client()
	if err != nil {
		return err
	}
	if err := client.Delete(ctx, syncDevicesPath+"/"+device.String(), ""); err != nil {
		return err
	}
	if device.String() == cli.Profile.Device {
		cli.Profile.Device, cli.Profile.Clock = "", ""
		if err := SaveProfile(cli.ProfilePath, cli.Profile); err != nil {
			return err
		}
	}
	printf(cli.Err, "forgot device %s\n", device)
	return nil
}
