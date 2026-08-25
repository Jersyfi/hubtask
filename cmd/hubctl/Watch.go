// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	port "github.com/Jersyfi/hubtask/core/port/httpclient"
)

const streamPath = "/stream"

const (
	// watchIdleLimit is how long the stream may stay silent before the connection is judged
	// dead. The server sends a heartbeat every 20 seconds (presentation/rest/StreamController.go);
	// four missed ones and a margin is a connection nobody is coming back for. This is what
	// bounds a read that the protocol deliberately leaves unbounded (rule 7).
	watchIdleLimit = 90 * time.Second
	// watchReconnectDelay is the pause before reconnecting when the server has not suggested one.
	// The server usually has: its retry field arrives with the stream and replaces this.
	watchReconnectDelay = 3 * time.Second
	// watchConnectAttempts bounds *consecutive* failures to connect. A stream that keeps ending
	// after it delivered something is a rolling update and worth following through; an address
	// that never answers is not going to start.
	watchConnectAttempts = 5
	// watchProblemBytes bounds how much of a refusal is read. A problem document is under a
	// kilobyte; the cap only exists because this read bypasses the transport's own.
	watchProblemBytes = 64 << 10
	// watchLineLimit bounds one line of the stream. An event carries one change record; this is
	// far past any record the server frames and still a bound.
	watchLineLimit = 1 << 20
)

func watchGroup() group {
	return group{
		name:      "watch",
		summary:   "follow the change stream as it happens",
		usage:     "[--from <cursor>]",
		unbounded: true,
		run:       watchRun,
	}
}

func watchRun(ctx context.Context, cli *CLI, args []string) error {
	flags := commandFlags(cli, "watch", "", "[--from <cursor>]")
	from := flags.String("from", "",
		"the cursor to resume from, as the stream last sent it; unset starts from now")
	if err := parseCommand(flags, args); err != nil {
		return err
	}
	if flags.NArg() > 0 {
		return usagef("unexpected argument %q: hubctl watch takes only flags", flags.Arg(0))
	}

	client, err := cli.client()
	if err != nil {
		return err
	}
	follower := &watcher{cli: cli, client: client, cursor: *from, delay: watchReconnectDelay}
	return follower.follow(ctx)
}

// watcher is one `hubctl watch`: the connection it holds, the cursor it stands at, and the pace
// it reconnects with.
type watcher struct {
	cli    *CLI
	client *Client
	cursor string
	delay  time.Duration

	headerShown bool
}

// follow keeps the stream alive until the user leaves. Ctrl-C is the intended end and exits
// clean: the context's cancellation reaches a blocked read by closing the body, and everything
// after that is silence rather than an error nobody caused.
func (w *watcher) follow(ctx context.Context) error {
	failures := 0
	for {
		connected, err := w.once(ctx)
		if ctx.Err() != nil {
			return nil
		}
		if err != nil {
			return err
		}
		if connected {
			failures = 0
		} else {
			failures++
			if failures >= watchConnectAttempts {
				return errorString("the stream cannot be reached; giving up after " +
					strconv.Itoa(failures) + " attempts")
			}
		}
		printf(w.cli.Err, "hubctl: the stream ended; reconnecting\n")
		if !waitOrGiveUp(ctx, w.delay) {
			return nil
		}
	}
}

// once holds one connection for as long as it lives. It reports whether the stream was reached at
// all - which is what resets the give-up counter - and returns an error only for a refusal that
// reconnecting cannot fix.
func (w *watcher) once(ctx context.Context) (bool, error) {
	response, err := w.client.OpenStream(ctx, w.cursor)
	if err != nil {
		if ctx.Err() != nil {
			return false, nil
		}
		// A transport failure is what reconnecting exists for; saying it and trying again is the
		// whole strategy.
		printf(w.cli.Err, "hubctl: %s\n", err)
		return false, nil
	}
	defer func() { _ = response.Body.Close() }()

	if response.Status == http.StatusServiceUnavailable {
		// Over the stream cap, or a pod that is shedding load. The server names the wait.
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, watchProblemBytes))
		w.delay = retryAfter(response.Header)
		return false, nil
	}
	if response.Status >= http.StatusBadRequest {
		// A cursor too old, a credential without the scope: reconnecting with the same request
		// would earn the same answer, so this ends the command with the catalogue's sentence.
		body, _ := io.ReadAll(io.LimitReader(response.Body, watchProblemBytes))
		return false, w.client.problem(port.Response{
			Status: response.Status, Header: response.Header, Body: body,
		})
	}

	w.read(ctx, response.Body)
	return true, nil
}

// read consumes the stream line by line until it ends.
//
// Two things can end a read that would otherwise block for ever, and both work by closing the
// body under the reader: the context being cancelled (Ctrl-C), and the idle watchdog judging the
// connection dead after the heartbeats stopped arriving.
func (w *watcher) read(ctx context.Context, body io.ReadCloser) {
	stopOnCancel := context.AfterFunc(ctx, func() { _ = body.Close() })
	defer stopOnCancel()
	watchdog := time.AfterFunc(watchIdleLimit, func() { _ = body.Close() })
	defer watchdog.Stop()

	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 4096), watchLineLimit)

	var event sseEvent
	for scanner.Scan() {
		watchdog.Reset(watchIdleLimit)
		line := scanner.Text()

		switch {
		case line == "":
			// The blank line dispatches. A frame without data - the retry line travels alone - is
			// bookkeeping, not an event.
			if len(event.data) > 0 {
				if event.id != "" {
					w.cursor = event.id
				}
				w.emit(event)
			}
			event = sseEvent{}
		case strings.HasPrefix(line, ":"):
			// A comment: the heartbeat, and the server's goodbye. Resetting the watchdog above is
			// all a keep-alive is for.
		default:
			event.field(line)
			if event.retry > 0 {
				w.delay = time.Duration(event.retry) * time.Millisecond
			}
		}
	}
}

// sseEvent is one frame of the stream, as the specification cuts it: fields accumulate until a
// blank line dispatches them.
type sseEvent struct {
	id    string
	name  string
	data  []string
	retry int
}

// field reads one `name: value` line. The specification says exactly one leading space of the
// value is stripped, and a line without a colon is a field with an empty value.
func (e *sseEvent) field(line string) {
	name, value, _ := strings.Cut(line, ":")
	value = strings.TrimPrefix(value, " ")
	switch name {
	case "id":
		e.id = value
	case "event":
		e.name = value
	case "data":
		e.data = append(e.data, value)
	case "retry":
		if parsed, err := strconv.Atoi(value); err == nil && parsed > 0 {
			e.retry = parsed
		}
	}
}

// emit prints one event. Under --json it is the record itself, one document per line, which is
// what a pipe can read as it arrives; the table form is one line per change.
func (w *watcher) emit(event sseEvent) {
	data := strings.Join(event.data, "\n")
	if w.cli.JSON {
		printf(w.cli.Out, "%s\n", data)
		return
	}

	var record struct {
		Entity      string    `json:"entity"`
		EntityID    string    `json:"entity_id"`
		Op          string    `json:"op"`
		OccurredAt  time.Time `json:"occurred_at"`
		ContainerID string    `json:"container_id"`
	}
	if err := json.Unmarshal([]byte(data), &record); err != nil {
		// A record that cannot be read is still a record; raw beats silently dropped.
		printf(w.cli.Out, "%s\n", data)
		return
	}
	if !w.headerShown {
		w.headerShown = true
		printf(w.cli.Out, "%-19s  %-14s  %-14s  %-36s  %s\n", "TIME", "ENTITY", "OP", "ID", "CONTAINER")
	}
	container := record.ContainerID
	if container == "" {
		container = "-"
	}
	occurred := record.OccurredAt
	printf(w.cli.Out, "%-19s  %-14s  %-14s  %-36s  %s\n",
		occurred.Local().Format("2006-01-02 15:04:05"), record.Entity, record.Op,
		record.EntityID, container)
}
