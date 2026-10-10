// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// replaceBatch is how many holds go in one call: the operation's own bound.
const replaceBatch = 1000

// holdSource is where the holds of a rewound period come from (backup-restore.md §8.5 step 5).
// Which source the procedure reads is still to be settled; this is the seam it plugs into, and the
// one reader built so far takes a file of hold records in the operation's own shape.
type holdSource interface {
	Holds(ctx context.Context) ([]openapi.LegalHoldRecord, error)
}

// fileHoldSource reads a JSON array of hold records, or an object carrying them under "holds".
type fileHoldSource struct{ path string }

func (s fileHoldSource) Holds(context.Context) ([]openapi.LegalHoldRecord, error) {
	raw, err := os.ReadFile(s.path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", s.path, err)
	}
	var records []openapi.LegalHoldRecord
	if err := json.Unmarshal(raw, &records); err == nil {
		return records, nil
	}
	var wrapped struct {
		Holds []openapi.LegalHoldRecord `json:"holds"`
	}
	if err := json.Unmarshal(raw, &wrapped); err != nil {
		return nil, fmt.Errorf("%s holds no hold records: %w", s.path, err)
	}
	return wrapped.Holds, nil
}

// adminTenantReplaceHolds places the holds of a rewound period again, in batches the operation
// accepts. Each batch is idempotent on the holds' identifiers, so a run cut short is run again.
func adminTenantReplaceHolds(ctx context.Context, cli *CLI, args []string) error {
	const usage = "admin tenant replace-holds <id> --recovery-point <time> --from <file>"
	tenantID, rest, err := cli.takeID(args, usage)
	if err != nil {
		return err
	}
	flags := commandFlags(cli, "admin tenant", "replace-holds", "<id> --recovery-point <time> --from <file>")
	recoveryPoint := flags.String("recovery-point", "", "the moment the installation was recovered to, RFC 3339")
	from := flags.String("from", "", "a file of the holds as they stood before the rewind")
	if err := parseCommand(flags, rest); err != nil {
		return err
	}
	if *recoveryPoint == "" || *from == "" {
		return usagef("admin tenant replace-holds needs --recovery-point and --from")
	}
	point, err := time.Parse(time.RFC3339, *recoveryPoint)
	if err != nil {
		return usagef("--recovery-point is not an RFC 3339 moment: %q", *recoveryPoint)
	}

	return replaceHolds(ctx, cli, tenantID.String(), point, fileHoldSource{path: *from})
}

func replaceHolds(
	ctx context.Context, cli *CLI, tenantID string, point time.Time, source holdSource,
) error {
	records, err := source.Holds(ctx)
	if err != nil {
		return err
	}
	if len(records) == 0 {
		return usagef("the source holds no legal hold to place again")
	}

	client, err := cli.client()
	if err != nil {
		return err
	}
	var all openapi.LegalHoldReplaceResult
	for start := 0; start < len(records); start += replaceBatch {
		end := min(start+replaceBatch, len(records))
		var answered openapi.LegalHoldReplaceResult
		if err := client.Post(ctx, adminTenantsPath+"/"+tenantID+":replace-legal-holds",
			openapi.LegalHoldReplaceRequest{RecoveryPoint: point, Holds: records[start:end]},
			&answered); err != nil {
			return err
		}
		all.Holds = append(all.Holds, answered.Holds...)
	}
	return cli.Emit(all, replacedTable(all))
}

// replacedTable says per hold what happened, and the two things the operator acts on: a hold
// whose target the recovery removed, and one released in the rewound period, whose owner releases
// it again.
func replacedTable(result openapi.LegalHoldReplaceResult) Table {
	rows := make([][]string, 0, len(result.Holds))
	for _, hold := range result.Holds {
		rows = append(rows, []string{
			hold.Id.String(), string(hold.Outcome),
			strconv.FormatBool(hold.TargetPresent), strconv.FormatBool(hold.ReleasedInPeriod),
		})
	}
	return Table{
		Columns: []string{"id", "outcome", "target present", "released in the period"},
		Rows:    rows,
	}
}
