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
	openapitypes "github.com/oapi-codegen/runtime/types"

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

// adminTenantReplaceHolds places the holds of a rewound period again. Holds read here go in batches
// the operation accepts, each idempotent on the holds' identifiers, so a run cut short is run
// again; an archive is read by the server, which holds its key, in one call.
func adminTenantReplaceHolds(ctx context.Context, cli *CLI, args []string) error {
	const sources = "(--from-cluster <dsn> | --from-archive <path> --target <id> | --from <file>)"
	const usage = "admin tenant replace-holds <id> --recovery-point <time> " + sources
	tenantID, rest, err := cli.takeID(args, usage)
	if err != nil {
		return err
	}
	flags := commandFlags(cli, "admin tenant", "replace-holds", "<id> --recovery-point <time> "+sources)
	recoveryPoint := flags.String("recovery-point", "", "the moment the installation was recovered to, RFC 3339")
	fromCluster := flags.String("from-cluster", "",
		"the connection string of the temporary cluster recovered to the end of the WAL archive; "+
			"its password belongs in PGPASSWORD or a password file, not here")
	fromArchive := flags.String("from-archive", "",
		"where the end of the WAL archive is unusable: the path of the workspace's newest archive "+
			"that includes the audit trail, as `hubctl backup ls` shows it")
	target := flags.String("target", "", "the backup target the archive lies at")
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

	named := 0
	for _, source := range []string{*fromCluster, *fromArchive, *from} {
		if source != "" {
			named++
		}
	}
	switch {
	case named == 0:
		return usagef("admin tenant replace-holds needs a source: %s", sources)
	case named > 1:
		return usagef("admin tenant replace-holds reads one source: %s", sources)
	case *fromArchive != "":
		if *target == "" {
			return usagef("--from-archive needs --target, the backup target the archive lies at")
		}
		targetID, err := cli.parseID("--target", *target)
		if err != nil {
			return err
		}
		return replaceFromArchive(ctx, cli, tenantID.String(), point, targetID, *fromArchive)
	case *target != "":
		return usagef("--target belongs to --from-archive")
	case *fromCluster != "":
		return replaceHolds(ctx, cli, tenantID.String(), point, clusterHoldSource{
			dsn: *fromCluster, tenantID: shared.ID(tenantID.String()), read: recoveredHolds,
		})
	default:
		return replaceHolds(ctx, cli, tenantID.String(), point, fileHoldSource{path: *from})
	}
}

func replaceHolds(
	ctx context.Context, cli *CLI, tenantID string, point time.Time, source holdSource,
) error {
	records, err := source.Holds(ctx)
	if err != nil {
		return err
	}
	var all openapi.LegalHoldReplaceResult
	if len(records) == 0 {
		// Nothing was placed after the recovery point, or nothing at all: an answer, not a mistake.
		printf(cli.Err, "the source holds no legal hold of workspace %s; nothing to place again\n", tenantID)
		return cli.Emit(all, replacedTable(all))
	}

	client, err := cli.client()
	if err != nil {
		return err
	}
	for start := 0; start < len(records); start += replaceBatch {
		end := min(start+replaceBatch, len(records))
		batch := records[start:end]
		var answered openapi.LegalHoldReplaceResult
		if err := client.Post(ctx, adminTenantsPath+"/"+tenantID+":replace-legal-holds",
			openapi.LegalHoldReplaceRequest{RecoveryPoint: point, Holds: &batch},
			&answered); err != nil {
			return err
		}
		all.Holds = append(all.Holds, answered.Holds...)
	}
	return cli.Emit(all, replacedTable(all))
}

// replaceFromArchive has the server read the archive and place its holds, and says where the gap
// begins: a hold placed after the archive was taken is not in it, and the operator reports that
// per workspace (backup-restore.md §8.5 step 5).
func replaceFromArchive(
	ctx context.Context, cli *CLI, tenantID string, point time.Time, targetID openapitypes.UUID, path string,
) error {
	client, err := cli.client()
	if err != nil {
		return err
	}
	request := openapi.LegalHoldReplaceRequest{RecoveryPoint: point}
	request.Archive = &struct {
		Path     string            `json:"path"`
		TargetId openapitypes.UUID `json:"target_id"` //nolint:revive // the generated field's name
	}{Path: path, TargetId: targetID}
	var answered openapi.LegalHoldReplaceResult
	if err := client.Post(ctx, adminTenantsPath+"/"+tenantID+":replace-legal-holds", request, &answered); err != nil {
		return err
	}
	if from := answered.Archive; from != nil {
		printf(cli.Err, "workspace %s: the archive was taken at %s; a hold placed between then and the loss "+
			"is not in it\n", tenantID, from.SnapshotAt.UTC().Format(time.RFC3339))
		if !from.AuditTrail {
			printf(cli.Err, "workspace %s: the archive does not include the audit trail; only the holds "+
				"its own copy of the workspace carried were read\n", tenantID)
		}
		if from.UnreadableEntries > 0 {
			printf(cli.Err, "workspace %s: %d record(s) of a hold named too little to place it again\n",
				tenantID, from.UnreadableEntries)
		}
	}
	return cli.Emit(answered, replacedTable(answered))
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
