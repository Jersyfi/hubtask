// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"context"

	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// The installation's own backup targets (multi-tenancy.md §5): the targets that belong to no
// workspace, where an operator exports a workspace that has none of its own. The same flags as a
// workspace's `backup target`, the control plane's paths, and a fresh step-up for the two writes.

const adminTargetsPath = "/admin/backup-targets"

func adminTarget(ctx context.Context, cli *CLI, args []string) error {
	const verbs = "ls, add, test, rm"
	if len(args) == 0 {
		return usagef("admin target needs a command: %s", verbs)
	}
	switch args[0] {
	case "ls":
		return adminTargetList(ctx, cli, args[1:])
	case "add":
		return adminTargetAdd(ctx, cli, args[1:])
	case "test":
		return adminTargetTest(ctx, cli, args[1:])
	case "rm":
		return adminTargetRemove(ctx, cli, args[1:])
	default:
		return usagef("admin target has no command %q: %s", args[0], verbs)
	}
}

func adminTargetList(ctx context.Context, cli *CLI, args []string) error {
	flags := commandFlags(cli, "admin target", "ls", "")
	if err := parseCommand(flags, args); err != nil {
		return err
	}
	client, err := cli.client()
	if err != nil {
		return err
	}
	var targets []openapi.BackupTarget
	if err := client.Get(ctx, adminTargetsPath, nil, &targets); err != nil {
		return err
	}
	return cli.Emit(targets, targetTable(targets))
}

func adminTargetAdd(ctx context.Context, cli *CLI, args []string) error {
	create, err := targetCreateFrom(cli, "admin target", args)
	if err != nil {
		return err
	}
	client, err := cli.client()
	if err != nil {
		return err
	}
	var target openapi.BackupTarget
	if err := cli.proveAgain(ctx, client, func(stepUp string) error {
		return client.PostWithHeader(ctx, adminTargetsPath, create, stepUpProof(stepUp), &target)
	}); err != nil {
		return err
	}
	return cli.Emit(target, targetTable([]openapi.BackupTarget{target}))
}

func adminTargetTest(ctx context.Context, cli *CLI, args []string) error {
	const usage = "admin target test <id>"
	targetID, rest, err := cli.takeID(args, usage)
	if err != nil {
		return err
	}
	flags := commandFlags(cli, "admin target", "test", "<id>")
	if err := parseOnlyFlags(flags, rest, usage); err != nil {
		return err
	}
	client, err := cli.client()
	if err != nil {
		return err
	}
	var probe openapi.BackupTargetProbe
	if err := client.Post(ctx, adminTargetsPath+"/"+targetID.String()+":test", nil, &probe); err != nil {
		return err
	}
	return emitProbe(cli, targetID.String(), probe)
}

func adminTargetRemove(ctx context.Context, cli *CLI, args []string) error {
	const usage = "admin target rm <id>"
	targetID, rest, err := cli.takeID(args, usage)
	if err != nil {
		return err
	}
	flags := commandFlags(cli, "admin target", "rm", "<id>")
	if err := parseOnlyFlags(flags, rest, usage); err != nil {
		return err
	}
	client, err := cli.client()
	if err != nil {
		return err
	}
	if err := cli.proveAgain(ctx, client, func(stepUp string) error {
		return client.DeleteWithHeader(ctx, adminTargetsPath+"/"+targetID.String(), stepUpProof(stepUp))
	}); err != nil {
		return err
	}
	return adminTargetList(ctx, cli, nil)
}
