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

	"github.com/google/uuid"

	domain "github.com/Jersyfi/hubtask/core/domain/model/lifecycle"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// replaceBatch is how many holds go in one call: the operation's own bound.
const replaceBatch = 1000

// holdSource is where the holds of a rewound period come from (backup-restore.md §8.5 step 5): the
// temporary cluster recovered to the end of the WAL archive, or a file of hold records in the
// operation's own shape.
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

// clusterHoldSource reads the workspace's holds from the temporary cluster the operator recovered
// to the end of the WAL archive. Read here rather than by the server: the connection string is the
// operator's credential to a second database, and a server that connected wherever a request said
// would reach any address it was given.
type clusterHoldSource struct {
	dsn      string
	tenantID shared.ID
	read     readRecoveredHolds
}

// readRecoveredHolds is how the temporary cluster is read.
type readRecoveredHolds func(
	ctx context.Context, dsn string, tenantID shared.ID, timeout time.Duration,
) ([]domain.LegalHold, error)

// recoveredHolds is the reader the command uses; the command's tests replace it, having no
// cluster to read.
var recoveredHolds readRecoveredHolds = postgres.ReadRecoveredHolds

func (s clusterHoldSource) Holds(ctx context.Context) ([]openapi.LegalHoldRecord, error) {
	holds, err := s.read(ctx, s.dsn, s.tenantID, postgres.RecoveredHoldsTimeout)
	if err != nil {
		return nil, err
	}
	records := make([]openapi.LegalHoldRecord, 0, len(holds))
	for _, hold := range holds {
		record, err := recordOf(hold)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, nil
}

// recordOf is one hold as the operation takes it: every value as it was, the release included,
// which the server reports rather than carries over.
func recordOf(hold domain.LegalHold) (openapi.LegalHoldRecord, error) {
	var record openapi.LegalHoldRecord
	var err error
	if record.Id, err = uuid.Parse(hold.ID.String()); err != nil {
		return record, fmt.Errorf("a hold's identifier: %w", err)
	}
	if record.PlacedBy, err = uuid.Parse(hold.PlacedBy.String()); err != nil {
		return record, fmt.Errorf("hold %s names no placer: %w", hold.ID, err)
	}
	record.Scope.Kind = openapi.LegalHoldRecordScopeKind(hold.Scope)
	if !hold.ScopeID.IsZero() {
		scopeID, err := uuid.Parse(hold.ScopeID.String())
		if err != nil {
			return record, fmt.Errorf("hold %s names no target: %w", hold.ID, err)
		}
		record.Scope.Id = &scopeID
	}
	record.Reason = hold.Reason
	record.PlacedAt = hold.PlacedAt.UTC()
	if !hold.ReleasedAt.IsZero() {
		released := hold.ReleasedAt.UTC()
		record.ReleasedAt = &released
	}
	return record, nil
}

// adminTenantReplaceHolds places the holds of a rewound period again, in batches the operation
// accepts. Each batch is idempotent on the holds' identifiers, so a run cut short is run again.
func adminTenantReplaceHolds(ctx context.Context, cli *CLI, args []string) error {
	const usage = "admin tenant replace-holds <id> --recovery-point <time> (--from-cluster <dsn> | --from <file>)"
	tenantID, rest, err := cli.takeID(args, usage)
	if err != nil {
		return err
	}
	flags := commandFlags(cli, "admin tenant", "replace-holds",
		"<id> --recovery-point <time> (--from-cluster <dsn> | --from <file>)")
	recoveryPoint := flags.String("recovery-point", "", "the moment the installation was recovered to, RFC 3339")
	fromCluster := flags.String("from-cluster", "",
		"the connection string of the temporary cluster recovered to the end of the WAL archive; "+
			"its password belongs in PGPASSWORD or a password file, not here")
	from := flags.String("from", "", "a file of the holds as they stood before the rewind")
	if err := parseCommand(flags, rest); err != nil {
		return err
	}
	if *recoveryPoint == "" {
		return usagef("admin tenant replace-holds needs --recovery-point")
	}
	point, err := time.Parse(time.RFC3339, *recoveryPoint)
	if err != nil {
		return usagef("--recovery-point is not an RFC 3339 moment: %q", *recoveryPoint)
	}

	var source holdSource
	switch {
	case *fromCluster != "" && *from != "":
		return usagef("admin tenant replace-holds reads one source: --from-cluster or --from")
	case *fromCluster != "":
		source = clusterHoldSource{
			dsn: *fromCluster, tenantID: shared.ID(tenantID.String()), read: recoveredHolds,
		}
	case *from != "":
		source = fileHoldSource{path: *from}
	default:
		return usagef("admin tenant replace-holds needs a source: --from-cluster or --from")
	}
	return replaceHolds(ctx, cli, tenantID.String(), point, source)
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
