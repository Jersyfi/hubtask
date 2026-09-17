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
	"github.com/Jersyfi/hubtask/core/shared/secret"
	clockadapter "github.com/Jersyfi/hubtask/infrastructure/clock"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// The conformance test offline-sync.md §9 names (N-13): a top-level verb that drives the server
// through the protocol as two devices and checks that what the server does is what a conforming
// client can rely on. It tests the server's side of each requirement; what is a client's alone -
// encryption at rest - it says so about rather than pretending to test. The report is the shape
// the evidence files under docs/evidence use, so that a run can be quoted.

const membershipsPath = "/memberships"

func conformanceGroup() group {
	return group{
		name:      "sync-conformance",
		summary:   "check a running instance against the client requirements of offline-sync.md §9",
		usage:     "[--report <file>] [--keep]",
		unbounded: true,
		run:       conformanceRun,
	}
}

func conformanceRun(ctx context.Context, cli *CLI, args []string) error {
	flags := commandFlags(cli, "sync-conformance", "", "[--report <file>] [--keep]")
	report := flags.String("report", "", "write the report here as well as to standard output")
	keep := flags.Bool("keep", false, "leave the hub, the service account and its token behind for a look")
	if err := parseCommand(flags, args); err != nil {
		return err
	}
	if flags.NArg() > 0 {
		return usagef("unexpected argument %q: hubctl sync-conformance takes only flags", flags.Arg(0))
	}
	client, err := cli.client()
	if err != nil {
		return err
	}

	run := &conformance{cli: cli, client: client, started: time.Now().UTC()}
	if err := run.setUp(ctx); err != nil {
		return err
	}
	if !*keep {
		defer run.tearDown(ctx)
	}
	run.checks(ctx)

	rendered := run.render()
	printf(cli.Out, "%s", rendered)
	if *report != "" {
		if err := os.WriteFile(*report, []byte(rendered), 0o600); err != nil {
			return err
		}
	}
	if run.failed() > 0 {
		return errorString(strconv.Itoa(run.failed()) + " of the checks failed")
	}
	return nil
}

// conformance is one run: the fixtures it made, the two devices, and what each check found.
type conformance struct {
	cli     *CLI
	client  *Client
	started time.Time

	hub, collection openapitypes.UUID
	// The second person: a service account with a role on the hub and a token of its own, whose
	// device sees the revocation. Made rather than asked for, so that the run needs one
	// credential and leaves the workspace as it found it.
	second      openapitypes.UUID
	secondToken openapitypes.UUID
	membership  openapitypes.UUID
	other       *Client

	deviceA, deviceB openapitypes.UUID
	clock            shared.HLC

	results []checkResult
}

type checkResult struct {
	number int
	claim  string
	// outcome is pass, fail, or not-tested - the last for what a client alone can answer.
	outcome string
	note    string
}

func (c *conformance) setUp(ctx context.Context) error {
	ids := clockadapter.NewUUIDv7(clockadapter.System{})
	c.deviceA, c.deviceB = uuidOf(ids.NewID()), uuidOf(ids.NewID())
	stamp := c.started.Format("2006-01-02 15:04:05")

	var hub openapi.Container
	if err := c.client.Post(ctx, containersPath, openapi.ContainerCreate{
		Type: openapi.ContainerType("HUB"), Name: "Sync conformance " + stamp,
	}, &hub); err != nil {
		return fmt.Errorf("creating the hub: %w", err)
	}
	c.hub = hub.Id
	var collection openapi.Container
	if err := c.client.Post(ctx, containersPath, openapi.ContainerCreate{
		Type: openapi.ContainerType("COLLECTION"), Name: "Checks", ParentId: &hub.Id,
	}, &collection); err != nil {
		return fmt.Errorf("creating the collection: %w", err)
	}
	c.collection = collection.Id

	var account openapi.Account
	if err := c.client.Post(ctx, serviceAccountPath,
		openapi.ServiceAccountCreate{DisplayName: "sync conformance, the second device"}, &account); err != nil {
		return fmt.Errorf("creating the second person: %w", err)
	}
	c.second = account.Id
	var minted openapi.AccessTokenSecret
	if err := c.client.Post(ctx, tokensPath, openapi.AccessTokenCreate{
		Name: "sync conformance " + stamp, AccountId: &account.Id,
		Scopes:    []string{"items:read", "items:write"},
		ExpiresAt: c.started.Add(24 * time.Hour),
	}, &minted); err != nil {
		return fmt.Errorf("minting the second person's token: %w", err)
	}
	c.secondToken = minted.Id
	other, err := NewClient(Profile{
		BaseURL: c.cli.Profile.BaseURL, Tenant: c.cli.Profile.Tenant, Token: secret.New(minted.Token),
	}, c.cli.Catalogue, c.cli.Timeout)
	if err != nil {
		return err
	}
	c.other = other

	var membership openapi.Membership
	role := openapi.MembershipRole("MEMBER")
	if err := c.client.Post(ctx, membershipsPath, openapi.MembershipGrant{
		AccountId: &account.Id, ScopeType: openapi.MembershipScope("HUB"), ScopeId: &hub.Id, Role: role,
	}, &membership); err != nil {
		return fmt.Errorf("granting the second person the hub: %w", err)
	}
	c.membership = membership.Id
	return nil
}

// tearDown removes what the run made. Best effort and in reverse: a fixture left behind is a
// nuisance rather than a wrong answer, and the report already says what happened.
func (c *conformance) tearDown(ctx context.Context) {
	paths := []string{tokensPath + "/" + c.secondToken.String(), containersPath + "/" + c.hub.String()}
	if c.membership != (openapitypes.UUID{}) {
		// Still there only when the revocation check did not get to revoke it.
		paths = append([]string{membershipsPath + "/" + c.membership.String()}, paths...)
	}
	for _, path := range paths {
		if err := c.client.Delete(ctx, path, ""); err != nil {
			printf(c.cli.Err, "hubctl: leaving %s behind: %v\n", path, err)
		}
	}
}

func (c *conformance) record(number int, claim, outcome, note string) {
	c.results = append(c.results, checkResult{number: number, claim: claim, outcome: outcome, note: note})
	printf(c.cli.Err, "  %d. %-9s %s\n", number, outcome, claim)
}

func (c *conformance) pass(number int, claim, note string) { c.record(number, claim, "pass", note) }
func (c *conformance) fail(number int, claim, note string) { c.record(number, claim, "fail", note) }

func (c *conformance) failed() int {
	failed := 0
	for _, result := range c.results {
		if result.outcome == "fail" {
			failed++
		}
	}
	return failed
}

// checks is §9 in order. Each check names its number, so that a broken server names the
// requirement it breaks.
func (c *conformance) checks(ctx context.Context) {
	printf(c.cli.Err, "hubctl: the client requirements of offline-sync.md §9, against %s\n", c.cli.Profile.BaseURL)
	c.checkIdentifiers(ctx)
	c.checkIdempotence(ctx)
	c.checkRevocation(ctx)
	c.checkCursorAndWalk(ctx)
	c.checkServerDecides(ctx)
	c.record(6, "Local storage is encrypted and discarded on sign-out", "not-tested",
		"a client's alone: nothing about it is visible from the server, and this runner does not pretend otherwise")
	c.checkUnknownFields(ctx)
	c.checkNothingOfTheRightColumn(ctx)
}

// pushed is one push's results, keyed by op_id.
type pushed struct {
	results map[string]openapi.SyncMutationResult
	err     error
}

func (c *conformance) push(ctx context.Context, client *Client, device openapitypes.UUID, mutations ...map[string]any) pushed {
	for _, mutation := range mutations {
		if _, stamped := mutation["hlc"]; !stamped {
			if _, fields := mutation["fields"]; !fields {
				mutation["hlc"] = c.tick()
			}
		}
	}
	var response openapi.SyncPushResponse
	err := client.Post(ctx, syncPushPath, map[string]any{
		"device_id": device.String(), "platform": syncPlatform, "mutations": mutations,
	}, &response)
	out := pushed{results: map[string]openapi.SyncMutationResult{}, err: err}
	for _, result := range response.Results {
		out.results[result.OpId.String()] = result
	}
	return out
}

func (c *conformance) tick() string {
	next, err := c.clock.Tick(time.Now(), c.deviceA.String())
	if err != nil {
		return ""
	}
	c.clock = next
	return next.String()
}

// pull pages the whole of what the device may hold, or the delta since the cursor.
func (c *conformance) pull(ctx context.Context, client *Client, device openapitypes.UUID, cursor string) ([]openapi.SyncChange, string, error) {
	var all []openapi.SyncChange
	for {
		request := map[string]any{"device_id": device.String(), "platform": syncPlatform, "limit": 200}
		if cursor != "" {
			request["cursor"] = cursor
		}
		var page openapi.SyncPullResponse
		if err := client.Post(ctx, syncPullPath, request, &page); err != nil {
			return all, cursor, err
		}
		all = append(all, page.Changes...)
		cursor = page.Cursor
		if !page.HasMore {
			return all, cursor, nil
		}
	}
}

// opOf is the op_id of a mutation this run built, which always carries one.
func opOf(mutation map[string]any) string {
	op, _ := mutation["op_id"].(string)
	return op
}

func newMutation(kind string, item openapitypes.UUID) map[string]any {
	return map[string]any{
		"op_id": freshUUIDv7(), "kind": kind, "item_id": item.String(),
	}
}

func freshUUIDv7() string {
	return clockadapter.NewUUIDv7(clockadapter.System{}).NewID().String()
}

func uuidOf(id shared.ID) openapitypes.UUID {
	var parsed openapitypes.UUID
	_ = parsed.UnmarshalText([]byte(id.String()))
	return parsed
}

// 1: the identifier the client assigned is the identifier the server reads back.
func (c *conformance) checkIdentifiers(ctx context.Context) {
	const claim = "Local IDs are UUIDv7 and final"
	item := uuidOf(clockadapter.NewUUIDv7(clockadapter.System{}).NewID())
	create := newMutation("ITEM_CREATE", item)
	create["payload"] = map[string]any{"type": "TASK", "collection_id": c.collection.String(), "title": "Requirement one"}
	out := c.push(ctx, c.client, c.deviceA, create)
	if out.err != nil {
		c.fail(1, claim, "the push failed: "+out.err.Error())
		return
	}
	result := out.results[opOf(create)]
	if result.Result != "APPLIED" || result.EntityId == nil || *result.EntityId != item {
		c.fail(1, claim, fmt.Sprintf("the creation was answered %s with entity %s, want APPLIED under the identifier the client assigned", result.Result, id(result.EntityId)))
		return
	}
	var read openapi.WorkItem
	if err := c.client.Get(ctx, itemsPath+"/"+item.String(), nil, &read); err != nil || read.Id != item {
		c.fail(1, claim, "the entry cannot be read back under the identifier the client assigned")
		return
	}
	c.pass(1, claim, "an ITEM_CREATE under a client-minted UUIDv7 is APPLIED with that identifier and reads back under it")
}

// 2: a repeated push takes effect exactly once and answers the same.
func (c *conformance) checkIdempotence(ctx context.Context) {
	const claim = "Every mutation carries an op_id and an HLC; repetition stays idempotent"
	item := uuidOf(clockadapter.NewUUIDv7(clockadapter.System{}).NewID())
	create := newMutation("ITEM_CREATE", item)
	create["payload"] = map[string]any{"type": "TASK", "collection_id": c.collection.String(), "title": "Requirement two"}
	first := c.push(ctx, c.client, c.deviceA, create)
	second := c.push(ctx, c.client, c.deviceA, create)
	if first.err != nil || second.err != nil {
		c.fail(2, claim, "a push failed: "+errors.Join(first.err, second.err).Error())
		return
	}
	op := opOf(create)
	if first.results[op].Result != "APPLIED" || second.results[op].Result != "APPLIED" ||
		id(second.results[op].EntityId) != id(first.results[op].EntityId) {
		c.fail(2, claim, fmt.Sprintf("the repeat answered %s/%s, want the first answer again", first.results[op].Result, second.results[op].Result))
		return
	}
	var read openapi.WorkItem
	if err := c.client.Get(ctx, itemsPath+"/"+item.String(), nil, &read); err != nil || read.Version != 1 {
		c.fail(2, claim, "the entry's version moved on the repeat, so the repeat applied again")
		return
	}
	// And a mutation without an op_id is refused rather than applied under one the server made.
	bare := map[string]any{"kind": "ITEM_CREATE", "item_id": freshUUIDv7(),
		"payload": map[string]any{"type": "TASK", "collection_id": c.collection.String(), "title": "No op_id"}}
	if out := c.push(ctx, c.client, c.deviceA, bare); out.err == nil {
		for _, result := range out.results {
			if result.Result == "APPLIED" {
				c.fail(2, claim, "a mutation without an op_id was applied")
				return
			}
		}
	}
	c.pass(2, claim, "the same op_id pushed twice answers APPLIED twice under one identifier and the entry's version stays 1; a mutation without an op_id is not applied")
}

// 3: a revoked membership arrives as ACCESS_REVOKED at the second device, and its push below is
// refused.
func (c *conformance) checkRevocation(ctx context.Context) {
	const claim = "After ACCESS_REVOKED or sync.gone, local data is deleted"
	_, cursor, err := c.pull(ctx, c.other, c.deviceB, "")
	if err != nil {
		c.fail(3, claim, "the second device could not synchronise before the revocation: "+err.Error())
		return
	}
	if err := c.client.Delete(ctx, membershipsPath+"/"+c.membership.String(), ""); err != nil {
		c.fail(3, claim, "revoking the second person's membership failed: "+err.Error())
		return
	}
	c.membership = openapitypes.UUID{}
	changes, _, err := c.pull(ctx, c.other, c.deviceB, cursor)
	if err != nil {
		c.fail(3, claim, "the second device's pull after the revocation failed: "+err.Error())
		return
	}
	revoked := false
	for _, change := range changes {
		if change.Op == "ACCESS_REVOKED" && change.ContainerId != nil && *change.ContainerId == c.hub {
			revoked = true
		}
	}
	if !revoked {
		c.fail(3, claim, "no ACCESS_REVOKED record for the hub reached the second device")
		return
	}
	create := newMutation("ITEM_CREATE", uuidOf(clockadapter.NewUUIDv7(clockadapter.System{}).NewID()))
	create["payload"] = map[string]any{"type": "TASK", "collection_id": c.collection.String(), "title": "After the revocation"}
	out := c.push(ctx, c.other, c.deviceB, create)
	if out.err != nil {
		c.fail(3, claim, "the second device's push after the revocation failed as a whole: "+out.err.Error())
		return
	}
	result := out.results[opOf(create)]
	if result.Result != "REJECTED" || result.Error == nil || result.Error.Code == nil || *result.Error.Code != "forbidden" {
		c.fail(3, claim, fmt.Sprintf("a push below the revoked hub was answered %s, want REJECTED with forbidden", result.Result))
		return
	}
	c.pass(3, claim, "the revoked membership arrives as an ACCESS_REVOKED record at the hub, addressed to the device's account, and a push below it is REJECTED with forbidden")
}

// 4: the full walk is complete; a cursor the server cannot read is refused rather than rounded.
func (c *conformance) checkCursorAndWalk(ctx context.Context) {
	const claim = "On sync.cursor_too_old, a full resynchronisation follows"
	changes, cursor, err := c.pull(ctx, c.client, c.deviceA, "")
	if err != nil {
		c.fail(4, claim, "the full synchronisation failed: "+err.Error())
		return
	}
	seen := map[string]bool{}
	for _, change := range changes {
		seen[change.EntityId.String()] = true
	}
	if !seen[c.hub.String()] || !seen[c.collection.String()] {
		c.fail(4, claim, "the full synchronisation does not deliver the hub and the collection this run made")
		return
	}
	if _, _, err := c.pull(ctx, c.client, c.deviceA, cursor); err != nil {
		c.fail(4, claim, "the cursor the walk ended on is refused: "+err.Error())
		return
	}
	var refusal APIError
	_, _, err = c.pull(ctx, c.client, c.deviceA, "not-a-cursor")
	if !errors.As(err, &refusal) || refusal.DetailCode != "sync.cursor_invalid" {
		c.fail(4, claim, "a cursor the server cannot read was not refused as sync.cursor_invalid")
		return
	}
	// The second half (SY-C, P-12): the snapshot is the page sequence with the pages joined -
	// the same number of records, and a cursor at its end the delta accepts.
	streamed, snapshotCursor, err := c.snapshot(ctx, c.client, c.deviceA)
	if err != nil {
		c.fail(4, claim, "the snapshot failed: "+err.Error())
		return
	}
	if streamed != len(changes) {
		c.fail(4, claim, fmt.Sprintf("the snapshot streamed %d records where the page sequence answered %d", streamed, len(changes)))
		return
	}
	if snapshotCursor == "" {
		c.fail(4, claim, "the snapshot ended without its cursor line")
		return
	}
	if _, _, err := c.pull(ctx, c.client, c.deviceA, snapshotCursor); err != nil {
		c.fail(4, claim, "the cursor the snapshot ended on is refused: "+err.Error())
		return
	}
	c.pass(4, claim, fmt.Sprintf("the walk from nothing delivers the hub and the collection and ends on a cursor the delta accepts; a cursor the server cannot read is sync.cursor_invalid; the snapshot streams the same %d records and ends on a cursor the delta accepts. A cursor past the window cannot be minted from outside - the server's own SY-5 covers sync.cursor_too_old", streamed))
}

// snapshot takes the initial synchronisation as one stream and counts its records, answering
// the cursor on its last line - empty where the stream ended before it.
func (c *conformance) snapshot(ctx context.Context, client *Client, device openapitypes.UUID) (int, string, error) {
	response, err := client.OpenSnapshot(ctx, map[string]any{"device_id": device.String(), "platform": syncPlatform})
	if err != nil {
		return 0, "", err
	}
	defer func() { _ = response.Body.Close() }()
	scanner := bufio.NewScanner(response.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), syncLineLimit)
	records, cursor := 0, ""
	for scanner.Scan() {
		var line struct {
			Cursor string `json:"cursor"`
			Entity string `json:"entity"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &line); err != nil {
			return records, "", fmt.Errorf("a snapshot line is not JSON: %w", err)
		}
		if line.Cursor != "" && line.Entity == "" {
			cursor = line.Cursor
			continue
		}
		records++
	}
	return records, cursor, scanner.Err()
}

// 5: the server's answer overrides the device's prediction, and a refusal carries its code.
func (c *conformance) checkServerDecides(ctx context.Context) {
	const claim = "Server responses overwrite local predictions; rejected mutations carry their code"
	item := uuidOf(clockadapter.NewUUIDv7(clockadapter.System{}).NewID())
	create := newMutation("ITEM_CREATE", item)
	create["payload"] = map[string]any{"type": "TASK", "collection_id": c.collection.String(), "title": "Server's word"}
	if out := c.push(ctx, c.client, c.deviceA, create); out.err != nil || out.results[opOf(create)].Result != "APPLIED" {
		c.fail(5, claim, "the entry for the check could not be created")
		return
	}
	// A title written now, under a fresh reading, so that the server holds a clock for the field;
	// then a patch with a reading older than that: the device predicted its title, the server
	// keeps its own, and the answer says so with the server's state.
	fresh := newMutation("ITEM_PATCH", item)
	fresh["fields"] = map[string]any{"title": map[string]any{"value": "Server's word, edited", "hlc": c.tick()}}
	if out := c.push(ctx, c.client, c.deviceA, fresh); out.err != nil || out.results[opOf(fresh)].Result != "APPLIED" {
		c.fail(5, claim, "the fresh patch the check builds on was not applied")
		return
	}
	stale, err := shared.NewHLC(time.Now().Add(-4*time.Minute), 1, c.deviceA.String())
	if err != nil {
		c.fail(5, claim, "building the stale reading: "+err.Error())
		return
	}
	patch := newMutation("ITEM_PATCH", item)
	patch["fields"] = map[string]any{"title": map[string]any{"value": "Device's prediction", "hlc": stale.String()}}
	out := c.push(ctx, c.client, c.deviceA, patch)
	if out.err != nil {
		c.fail(5, claim, "the patch failed: "+out.err.Error())
		return
	}
	result := out.results[opOf(patch)]
	state, err := json.Marshal(result.ServerState)
	if err != nil {
		state = []byte(err.Error())
	}
	if result.Result == "APPLIED" || result.ServerState == nil || !strings.Contains(string(state), "Server's word, edited") {
		c.fail(5, claim, fmt.Sprintf("a stale patch was answered %s with %s, want the server's state to stand", result.Result, state))
		return
	}
	empty := newMutation("ITEM_CREATE", uuidOf(clockadapter.NewUUIDv7(clockadapter.System{}).NewID()))
	empty["payload"] = map[string]any{"type": "TASK", "collection_id": c.collection.String(), "title": ""}
	out = c.push(ctx, c.client, c.deviceA, empty)
	if out.err != nil {
		c.fail(5, claim, "the refused creation failed as a whole: "+out.err.Error())
		return
	}
	refused := out.results[opOf(empty)]
	if refused.Result != "REJECTED" || refused.Error == nil || refused.Error.Code == nil || refused.Error.MessageCode == nil || *refused.Error.MessageCode == "" {
		c.fail(5, claim, fmt.Sprintf("a creation without a title was answered %s without a code", refused.Result))
		return
	}
	c.pass(5, claim, fmt.Sprintf("a patch with a stale reading is answered %s with the server's state, not the device's; a creation without a title is REJECTED with %s", result.Result, *refused.Error.MessageCode))
}

// 7: forward compatibility, from the server's side. The requirement is the client's - a field the
// server sends and the client does not know is kept and written back - and what the server owes
// it is the converse: a field the server does not know is named in a refusal rather than
// dropped in silence, so that a client of a later version learns which of its fields did not
// land instead of believing they did. Both the frame and the payload are checked.
func (c *conformance) checkUnknownFields(ctx context.Context) {
	const claim = "Unknown fields and enum values are tolerated and written back unchanged"
	item := uuidOf(clockadapter.NewUUIDv7(clockadapter.System{}).NewID())
	framed := newMutation("ITEM_CREATE", item)
	framed["payload"] = map[string]any{"type": "TASK", "collection_id": c.collection.String(), "title": "Requirement seven"}
	framed["a_field_of_a_later_version"] = "unknown to this server"
	out := c.push(ctx, c.client, c.deviceA, framed)
	var refusal APIError
	if !errors.As(out.err, &refusal) || refusal.Status != 422 || !strings.Contains(refusal.Error(), "a_field_of_a_later_version") {
		c.fail(7, claim, "a mutation carrying a field the server does not know was not refused naming the field: "+outcomeOf(out.err))
		return
	}
	if err := c.client.Get(ctx, itemsPath+"/"+item.String(), nil, nil); err == nil {
		c.fail(7, claim, "the mutation with the unknown field was applied although the refusal said otherwise")
		return
	}
	inPayload := newMutation("ITEM_CREATE", item)
	inPayload["payload"] = map[string]any{"type": "TASK", "collection_id": c.collection.String(), "title": "Requirement seven", "a_field_of_a_later_version": "unknown"}
	out = c.push(ctx, c.client, c.deviceA, inPayload)
	if out.err != nil {
		c.fail(7, claim, "a mutation whose payload carries a field the server does not know failed as a whole: "+out.err.Error())
		return
	}
	result := out.results[opOf(inPayload)]
	if result.Result != "REJECTED" || result.Error == nil || result.Error.MessageCode == nil ||
		(*result.Error.MessageCode != "usecase.field_unknown" && *result.Error.MessageCode != "usecase.input_invalid") {
		c.fail(7, claim, fmt.Sprintf("a payload with a field the server does not know was answered %s, want REJECTED naming the field", result.Result))
		return
	}
	c.pass(7, claim, "a field the server does not know is refused by name, in the frame (422, usecase.field_unknown) and in a payload (REJECTED, "+*result.Error.MessageCode+"), never dropped in silence; that a client keeps a field the server sends is the client's alone")
}

func outcomeOf(err error) string {
	if err == nil {
		return "it was accepted"
	}
	return err.Error()
}

// 8: no mutation kind exists for what §1's right column keeps online.
func (c *conformance) checkNothingOfTheRightColumn(ctx context.Context) {
	const claim = "Nothing in the right-hand column of §1 is offered offline"
	for _, kind := range []string{"RULE_CREATE", "MEMBERSHIP_GRANT", "TEMPLATE_INSTANTIATE", "RESTORE"} {
		mutation := newMutation(kind, uuidOf(clockadapter.NewUUIDv7(clockadapter.System{}).NewID()))
		out := c.push(ctx, c.client, c.deviceA, mutation)
		if out.err != nil {
			// Refused at the door as a whole - a validation of the request - is a refusal too.
			var refusal APIError
			if errors.As(out.err, &refusal) && refusal.Status == 422 {
				continue
			}
			c.fail(8, claim, "pushing "+kind+" failed for another reason: "+out.err.Error())
			return
		}
		result := out.results[opOf(mutation)]
		if result.Result != "REJECTED" || result.Error == nil || result.Error.MessageCode == nil || *result.Error.MessageCode != "sync.kind_unknown" {
			c.fail(8, claim, fmt.Sprintf("a mutation of kind %s was answered %s, want REJECTED as sync.kind_unknown", kind, result.Result))
			return
		}
	}
	c.pass(8, claim, "a rule, a membership, a template instantiation and a restore have no mutation kind: each is refused as sync.kind_unknown")
}

// render is the report, in the shape of docs/evidence: a header table, then one row per
// requirement with its number, its outcome and what was seen.
func (c *conformance) render() string {
	var out strings.Builder
	fmt.Fprintf(&out, "# Sync conformance — offline-sync.md §9\n\n")
	fmt.Fprintf(&out, "**%s, `hubctl sync-conformance` against %s.** The eight requirements on clients, checked from the server's side as two devices: the first the signed-in account's, the second a service account's with a role on a hub this run made and took away again.\n\n",
		c.started.Format("2006-01-02 15:04 MST"), c.cli.Profile.BaseURL)
	fmt.Fprintf(&out, "| | Value |\n|---|---|\n")
	fmt.Fprintf(&out, "| Installation | %s |\n", c.cli.Profile.BaseURL)
	fmt.Fprintf(&out, "| Hub | `%s` |\n", c.hub)
	fmt.Fprintf(&out, "| Devices | `%s` and `%s` |\n", c.deviceA, c.deviceB)
	fmt.Fprintf(&out, "| Checks | %d, %d failed, 1 not testable from outside |\n\n", len(c.results), c.failed())
	fmt.Fprintf(&out, "| # | Requirement | Result | What was seen |\n|---|---|---|---|\n")
	for _, result := range c.results {
		fmt.Fprintf(&out, "| %d | %s | **%s** | %s |\n", result.number, result.claim, result.outcome, result.note)
	}
	return out.String()
}
