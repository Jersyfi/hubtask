// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package harness

import (
	"bufio"
	"encoding/json"
	"io"
	"time"
)

// SnapshotRead is what one `sync:snapshot` response carried: its records, its bytes, when the
// first byte came, and whether it ended in the cursor line. A response without the cursor was
// cut short - by the deadline or the byte budget - and is not one a device may resume from.
type SnapshotRead struct {
	Records        int   `json:"records"`
	Bytes          int64 `json:"bytes"`
	FirstByteMS    int64 `json:"first_byte_ms"`
	CursorReceived bool  `json:"cursor_received"`
	// Malformed counts lines that were not one complete JSON object, which the contract never
	// sends.
	Malformed int `json:"malformed_lines"`
}

// ReadSnapshot reads an x-ndjson snapshot to its end. started is when the request was sent, so
// the first byte's delay includes the server's time before it wrote anything.
func ReadSnapshot(r io.Reader, started time.Time) (SnapshotRead, error) {
	var read SnapshotRead
	lines := bufio.NewScanner(r)
	lines.Buffer(make([]byte, 0, 64<<10), 16<<20)
	last := ""
	for lines.Scan() {
		line := lines.Bytes()
		if read.Bytes == 0 {
			read.FirstByteMS = time.Since(started).Milliseconds()
		}
		read.Bytes += int64(len(line)) + 1
		if len(line) == 0 {
			continue
		}
		var probe map[string]json.RawMessage
		if json.Unmarshal(line, &probe) != nil {
			read.Malformed++
			continue
		}
		if cursor, isCursor := probe["cursor"]; isCursor && len(probe) == 1 {
			last = string(cursor)
			continue
		}
		if last != "" {
			// A record after the cursor line: the cursor was not the last line after all.
			last = ""
		}
		read.Records++
	}
	read.CursorReceived = last != "" && last != `""` && last != "null"
	return read, lines.Err()
}
