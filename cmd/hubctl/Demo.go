// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"bytes"
	"compress/zlib"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"hash/crc32"
	"maps"
	"net/url"
	"slices"
	"strings"
	"time"
)

const (
	workspaceSamplesPath = workspacePath + "/samples"
	workspaceDemoUsage   = "--workspace <short> [--voices] [--remove]"
	// demoBudget bounds the whole showcase; each call keeps the CLI's own timeout (rule 7).
	demoBudget = 30 * time.Minute
)

// workspaceDemo fills the credential's workspace with the showcase (user-experience.md §3.6) as
// the signed-in person and marks every object it makes as sample content. A run again makes only
// what no mark holds yet; --remove takes it all away in one act. With --voices, what other people
// of scripts/seed-demo.sh say - their comments, time and check-ins, a requester's request - is
// not written but answered in order under `voices`, for seed-demo.sh to write as each of them.
func workspaceDemo(ctx context.Context, cli *CLI, args []string) error {
	flags := commandFlags(cli, "workspace", "demo", workspaceDemoUsage)
	short := flags.String("workspace", "", "the workspace's short name, as a guard against filling the wrong one")
	remove := flags.Bool("remove", false, "remove the whole showcase instead")
	voices := flags.Bool("voices", false, "answer what other people say instead of writing it as the signed-in person")
	if err := parseCommand(flags, args); err != nil {
		return err
	}
	if *short == "" {
		return usagef("--workspace names the workspace to fill: hubctl workspace demo %s", workspaceDemoUsage)
	}
	ctx, cancel := context.WithTimeout(ctx, demoBudget)
	defer cancel()
	client, err := cli.client()
	if err != nil {
		return err
	}
	d := &demo{cli: cli, c: client, held: map[string]string{}, people: map[string]string{}, groups: map[string]string{},
		counts: map[string]int{}, voices: *voices, today: time.Now().UTC().Truncate(24 * time.Hour)}
	var workspace map[string]any
	if err := d.call(ctx, func(ctx context.Context) error { return client.Get(ctx, workspacePath, nil, &workspace) }); err != nil {
		return err
	}
	if slug, _ := workspace["slug"].(string); slug != *short {
		return usagef("the credential belongs to the workspace %q, not %q", slug, *short)
	}
	if *remove {
		var out map[string]any
		if err := d.call(ctx, func(ctx context.Context) error { return client.Post(ctx, workspaceSamplesPath+":remove", nil, &out) }); err != nil {
			return err
		}
		rows := [][]string{{"removed", fmt.Sprint(out["removed"])}}
		if kept, _ := out["kept"].([]any); len(kept) > 0 {
			for _, k := range kept {
				row, _ := k.(map[string]any)
				rows = append(rows, []string{"kept " + str(row["key"]), str(row["code"])})
			}
		}
		return cli.Emit(out, Table{Columns: []string{"what", "count"}, Rows: rows})
	}
	if err := d.load(ctx); err != nil {
		return err
	}
	d.run(ctx)
	d.flush(ctx)
	rows := make([][]string, 0, len(d.counts)+len(d.refused))
	for _, kind := range slices.Sorted(maps.Keys(d.counts)) {
		rows = append(rows, []string{kind, fmt.Sprint(d.counts[kind])})
	}
	for _, refusal := range d.refused {
		rows = append(rows, []string{"refused", refusal})
	}
	if d.voices {
		rows = append(rows, []string{"voices", fmt.Sprint(len(d.spoken))})
	}
	if err := cli.Emit(map[string]any{"made": d.counts, "refused": d.refused, "marked": len(d.held), "voices": d.spoken},
		Table{Columns: []string{"what", "count"}, Rows: rows}); err != nil {
		return err
	}
	if len(d.refused) > 0 {
		return errorString(fmt.Sprintf("%d write(s) were refused; the showcase is incomplete - run it again once the cause is fixed", len(d.refused)))
	}
	return nil
}

// demo is one run of the showcase: the marks the workspace holds and what this run made.
type demo struct {
	cli     *CLI
	c       *Client
	held    map[string]string // mark key -> object id
	people  map[string]string // display name -> account id, where the workspace has the person
	self    string            // the signed-in person's account id, once read
	groups  map[string]string // group name -> id, where the workspace has the group
	counts  map[string]int
	refused []string
	today   time.Time
	voices  bool
	spoken  []demoVoice
	pending []demoMark
}

// demoVoice is one write left to its speaker; "" is the signed-in person, whose write waits for
// a voice before it. `{voice:N}` in its path or body stands for the identifier voice N made.
type demoVoice struct {
	Speaker string    `json:"speaker"`
	What    string    `json:"what"`
	Method  string    `json:"method"`
	Path    string    `json:"path"`
	Body    any       `json:"body,omitempty"`
	Mark    *demoMark `json:"mark,omitempty"`
}

// demoMark is a mark to write once the object is made, or, for a versioned kind, once the
// showcase's own follow-ups have moved its version.
type demoMark struct {
	Key  string `json:"key"`
	Kind string `json:"kind"`
	ID   string `json:"-"`
	Read string `json:"-"`
}

func (d *demo) call(ctx context.Context, fn func(context.Context) error) error {
	bounded, cancel := context.WithTimeout(ctx, d.cli.Timeout)
	defer cancel()
	return fn(bounded)
}

// load reads the marks the workspace holds and the people the showcase names, by name.
func (d *demo) load(ctx context.Context) error {
	var listed struct {
		Samples []struct {
			Key      string `json:"key"`
			ObjectID string `json:"object_id"`
		} `json:"samples"`
	}
	if err := d.call(ctx, func(ctx context.Context) error { return d.c.Get(ctx, workspaceSamplesPath, nil, &listed) }); err != nil {
		return err
	}
	for _, s := range listed.Samples {
		d.held[s.Key] = s.ObjectID
	}
	var me struct {
		ID string `json:"id"`
	}
	if err := d.call(ctx, func(ctx context.Context) error { return d.c.Get(ctx, "/accounts/me", nil, &me) }); err == nil {
		d.self = me.ID
	}
	var groups struct {
		Data []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"data"`
	}
	if err := d.call(ctx, func(ctx context.Context) error { return d.c.Get(ctx, "/groups", nil, &groups) }); err == nil {
		for _, g := range groups.Data {
			d.groups[g.Name] = g.ID
		}
	}
	var members struct {
		Data []struct {
			AccountID string `json:"account_id"`
		} `json:"data"`
	}
	query := url.Values{"scope_type": {"TENANT"}}
	if err := d.call(ctx, func(ctx context.Context) error { return d.c.Get(ctx, "/memberships", query, &members) }); err != nil {
		return nil //nolint:nilerr // without the list the showcase assigns nobody; it is still a showcase
	}
	ids := make([]string, 0, len(members.Data))
	for _, m := range members.Data {
		if m.AccountID != "" && len(ids) < 200 {
			ids = append(ids, m.AccountID)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	var resolved struct {
		Data []struct {
			ID          string `json:"id"`
			DisplayName string `json:"display_name"`
		} `json:"data"`
	}
	if err := d.call(ctx, func(ctx context.Context) error {
		return d.c.Post(ctx, "/accounts:resolve", map[string]any{"ids": ids}, &resolved)
	}); err == nil {
		for _, a := range resolved.Data {
			d.people[a.DisplayName] = a.ID
		}
	}
	return nil
}

// who is a person of scripts/seed-demo.sh where the workspace has them; nil leaves the field out.
func (d *demo) who(name string) any {
	if id, ok := d.people[name]; ok {
		return id
	}
	return nil
}

func (d *demo) fail(what string, err error) {
	d.refused = append(d.refused, what+": "+err.Error())
}

// object makes one object and marks it under key, or answers the object a mark holds already;
// fresh says this run made it, so that what belongs to it is written once.
func (d *demo) object(ctx context.Context, key, kind, label, path string, body map[string]any) (string, bool) {
	return d.spokenObject(ctx, "", key, kind, label, path, body)
}

// spokenObject is object, made by speaker where --voices leaves it to them.
func (d *demo) spokenObject(ctx context.Context, speaker, key, kind, label, path string, body map[string]any) (string, bool) {
	if id, held := d.held[key]; held {
		return id, false
	}
	if d.voices && (speaker != "" || waits(path, body)) {
		d.counts[label]++
		mark := &demoMark{Key: key, Kind: kind}
		if kind == "TIME_RECORD" && speaker != "" {
			// Another person's time is theirs alone (T-04): nobody else could remove it.
			mark = nil
		}
		return d.queue(demoVoice{Speaker: speaker, What: key, Method: "POST", Path: path, Body: compact(body), Mark: mark}), true
	}
	var out map[string]any
	if err := d.call(ctx, func(ctx context.Context) error { return d.c.Post(ctx, path, compact(body), &out) }); err != nil {
		d.fail(key, err)
		return "", false
	}
	id := str(out["id"])
	if id == "" {
		id = str(out["item_id"])
	}
	if id == "" {
		d.refused = append(d.refused, key+": the answer named no identifier")
		return "", false
	}
	if read, versioned := versionedSample[kind]; versioned {
		// The showcase's own check-ins and updates move the version; it is marked after them.
		d.pending = append(d.pending, demoMark{Key: key, Kind: kind, ID: id, Read: read + id})
		d.held[key] = id
	} else {
		d.mark(ctx, key, kind, id, out["version"])
	}
	d.counts[label]++
	return id, true
}

// versionedSample names where an object deleted only at its marked version is read.
var versionedSample = map[string]string{"GOAL": "/goals/", "PORTFOLIO": "/portfolios/", "DASHBOARD": "/dashboards/"}

// flush marks the versioned objects at the version they stand at now; under --voices, once the
// voices have been written (a MARK voice: read the path, mark what it answers).
func (d *demo) flush(ctx context.Context) {
	for _, m := range d.pending {
		if d.voices {
			d.queue(demoVoice{What: "mark " + m.Key, Method: "MARK", Path: m.Read, Mark: &demoMark{Key: m.Key, Kind: m.Kind}})
			continue
		}
		var now map[string]any
		if err := d.call(ctx, func(ctx context.Context) error { return d.c.Get(ctx, m.Read, nil, &now) }); err != nil {
			d.fail("mark "+m.Key, err)
			delete(d.held, m.Key)
			continue
		}
		delete(d.held, m.Key)
		d.mark(ctx, m.Key, m.Kind, m.ID, now["version"])
	}
	d.pending = nil
}

func (d *demo) queue(v demoVoice) string {
	d.spoken = append(d.spoken, v)
	return fmt.Sprintf("{voice:%d}", len(d.spoken)-1)
}

// waits reports whether a write names what a voice before it is still to make.
func waits(path string, body any) bool {
	if strings.Contains(path, "{voice:") {
		return true
	}
	raw, err := json.Marshal(body)
	return err == nil && strings.Contains(string(raw), "{voice:")
}

// say is a write by speaker: left to them under --voices, written by the signed-in person
// otherwise.
func (d *demo) say(ctx context.Context, speaker, what, method, path string, body any) map[string]any {
	if d.voices && speaker != "" {
		d.queue(demoVoice{Speaker: speaker, What: what, Method: method, Path: path, Body: body})
		return map[string]any{}
	}
	return d.write(ctx, what, method, path, body)
}

// later is the signed-in person's write that has to follow the voices before it.
func (d *demo) later(ctx context.Context, what, method, path string, body any) map[string]any {
	if d.voices {
		d.queue(demoVoice{What: what, Method: method, Path: path, Body: body})
		return map[string]any{}
	}
	return d.write(ctx, what, method, path, body)
}

func (d *demo) mark(ctx context.Context, key, kind, id string, version any) {
	mark := map[string]any{"key": key, "kind": kind, "object_id": id}
	if v, ok := version.(float64); ok {
		mark["version"] = int64(v)
	}
	if err := d.call(ctx, func(ctx context.Context) error { return d.c.Post(ctx, workspaceSamplesPath, mark, nil) }); err != nil {
		d.fail("mark "+key, err)
		return
	}
	d.held[key] = id
}

// write is one write that belongs to an object made in this run: a step, a link, a comment.
func (d *demo) write(ctx context.Context, what, method, path string, body any) map[string]any {
	if d.voices && waits(path, body) {
		d.queue(demoVoice{What: what, Method: method, Path: path, Body: body})
		return map[string]any{}
	}
	var out map[string]any
	err := d.call(ctx, func(ctx context.Context) error {
		switch method {
		case "PUT":
			return d.c.Put(ctx, path, body, &out)
		case "PATCH":
			return d.c.Patch(ctx, path, body, &out)
		case "GET":
			return d.c.Get(ctx, path, nil, &out)
		}
		return d.c.Post(ctx, path, body, &out)
	})
	if err != nil {
		d.fail(what, err)
		return nil
	}
	return out
}

func (d *demo) day(offset, hour int) string {
	return d.today.AddDate(0, 0, offset).Add(time.Duration(hour) * time.Hour).Format(time.RFC3339)
}

func (d *demo) date(offset int) string { return d.today.AddDate(0, 0, offset).Format(time.DateOnly) }

// monday is the Monday of the week the day `offset` days from today falls in, as an offset.
func (d *demo) monday(offset int) int {
	weekday := (int(d.today.AddDate(0, 0, offset).Weekday()) + 6) % 7
	return offset - weekday
}

// compact leaves out the empty fields of a body, so that only what the showcase has is sent.
func compact(body map[string]any) map[string]any {
	out := make(map[string]any, len(body))
	for k, v := range body {
		if v == nil {
			continue
		}
		if s, ok := v.(string); ok && s == "" {
			continue
		}
		out[k] = v
	}
	return out
}

func str(v any) string {
	s, _ := v.(string)
	return s
}

// png draws a small solid image, so that the showcase needs no file.
func png(width, height int, rgb [3]byte) []byte {
	row := append([]byte{0}, bytes.Repeat(rgb[:], width)...)
	var raw bytes.Buffer
	z := zlib.NewWriter(&raw)
	for range height {
		_, _ = z.Write(row)
	}
	_ = z.Close()
	var out bytes.Buffer
	out.WriteString("\x89PNG\r\n\x1a\n")
	chunk := func(kind string, data []byte) {
		_ = binary.Write(&out, binary.BigEndian, uint32(len(data))) //nolint:gosec // G115: a small image
		body := append([]byte(kind), data...)
		out.Write(body)
		_ = binary.Write(&out, binary.BigEndian, crc32.ChecksumIEEE(body))
	}
	header := make([]byte, 13)
	binary.BigEndian.PutUint32(header[0:], uint32(width))  //nolint:gosec // G115: a small image
	binary.BigEndian.PutUint32(header[4:], uint32(height)) //nolint:gosec // G115: a small image
	header[8], header[9] = 8, 2
	chunk("IHDR", header)
	chunk("IDAT", raw.Bytes())
	chunk("IEND", nil)
	return out.Bytes()
}

// upload stages, puts and confirms an image, as `hubctl media upload` does.
func (d *demo) upload(ctx context.Context, name string, data []byte) string {
	var staged struct {
		ID     string `json:"id"`
		Upload *struct {
			Method string `json:"method"`
			URL    string `json:"url"`
		} `json:"upload"`
	}
	body := map[string]any{"file_name": name, "content_type": "image/png", "size": len(data), "usage": "ATTACHMENT"}
	if err := d.call(ctx, func(ctx context.Context) error { return d.c.Post(ctx, mediaPath, body, &staged) }); err != nil {
		d.fail("upload "+name, err)
		return ""
	}
	if staged.Upload == nil {
		d.refused = append(d.refused, "upload "+name+": no target to put the bytes")
		return ""
	}
	target := staged.Upload.URL
	if strings.HasPrefix(target, "/") {
		target = d.cli.Profile.BaseURL + target
	}
	if err := d.call(ctx, func(ctx context.Context) error { return d.c.Upload(ctx, staged.Upload.Method, target, data) }); err != nil {
		d.fail("upload "+name, err)
		return ""
	}
	if d.write(ctx, "confirm "+name, "POST", mediaPath+"/"+staged.ID+":confirm", nil) == nil {
		return ""
	}
	d.counts["images"]++
	return staged.ID
}
