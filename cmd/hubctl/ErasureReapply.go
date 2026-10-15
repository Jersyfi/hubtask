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

	domain "github.com/Jersyfi/hubtask/core/domain/model/privacy"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// reapplyBatch is how many cases go in one call: the operation's own bound.
const reapplyBatch = 1000

// readRecoveredErasures is how the temporary cluster is read; the command's tests replace it,
// having no cluster to read.
type readRecoveredErasures func(
	ctx context.Context, dsn string, tenantID shared.ID, point time.Time, timeout time.Duration,
) ([]postgres.RecoveredErasure, error)

var recoveredErasures readRecoveredErasures = postgres.ReadRecoveredErasures

// unsent is a case the source could not name a subject for: reported, never sent - erasing the
// wrong person cannot be undone.
type unsent struct {
	id      string
	outcome string
}

// adminTenantReapplyErasures carries out again the erasures of a rewound period
// (backup-restore.md §8.5 step 5), after the holds are placed again. The cases are read here - from
// the temporary cluster recovered to the end of the WAL archive, or from a file of cases in the
// operation's own shape - and sent in batches the operation accepts, each idempotent, each behind
// its own step-up: the operation deletes for good.
func adminTenantReapplyErasures(ctx context.Context, cli *CLI, args []string) error {
	const sources = "(--from-cluster <dsn> | --from <file>)"
	const usage = "admin tenant reapply-erasures <id> --recovery-point <time> " + sources
	tenantID, rest, err := cli.takeID(args, usage)
	if err != nil {
		return err
	}
	flags := commandFlags(cli, "admin tenant", "reapply-erasures", "<id> --recovery-point <time> "+sources)
	recoveryPoint := flags.String("recovery-point", "", "the moment the installation was recovered to, RFC 3339")
	fromCluster := flags.String("from-cluster", "",
		"the connection string of the temporary cluster recovered to the end of the WAL archive; "+
			"its password belongs in PGPASSWORD or a password file, not here")
	from := flags.String("from", "", "a file of the erasure cases completed after the recovery point")
	if err := parseCommand(flags, rest); err != nil {
		return err
	}
	if *recoveryPoint == "" {
		return usagef("admin tenant reapply-erasures needs --recovery-point")
	}
	point, err := time.Parse(time.RFC3339, *recoveryPoint)
	if err != nil {
		return usagef("--recovery-point is not an RFC 3339 moment: %q", *recoveryPoint)
	}

	var records []openapi.ErasureCaseRecord
	var skipped []unsent
	switch {
	case *fromCluster != "" && *from != "":
		return usagef("admin tenant reapply-erasures reads one source: %s", sources)
	case *fromCluster != "":
		read, err := recoveredErasures(ctx, *fromCluster, shared.ID(tenantID.String()), point,
			postgres.RecoveredErasuresTimeout)
		if err != nil {
			return err
		}
		if records, skipped, err = erasureRecordsOf(read); err != nil {
			return err
		}
	case *from != "":
		if records, err = erasureFile(*from); err != nil {
			return err
		}
	default:
		return usagef("admin tenant reapply-erasures needs a source: %s", sources)
	}

	var all openapi.ErasureReapplyResult
	if len(records) == 0 {
		printf(cli.Err, "the source holds no erasure of workspace %s completed after %s; nothing to carry out again\n",
			tenantID, point.UTC().Format(time.RFC3339))
		return cli.Emit(all, reappliedTable(all, skipped))
	}
	client, err := cli.client()
	if err != nil {
		return err
	}
	for start := 0; start < len(records); start += reapplyBatch {
		batch := records[start:min(start+reapplyBatch, len(records))]
		var answered openapi.ErasureReapplyResult
		err := cli.proveAgain(ctx, client, func(stepUp string) error {
			return client.PostWithHeader(ctx, adminTenantsPath+"/"+tenantID.String()+":reapply-erasures",
				openapi.ErasureReapplyRequest{RecoveryPoint: point, Erasures: batch}, stepUpProof(stepUp), &answered)
		})
		if err != nil {
			return err
		}
		all.Erasures = append(all.Erasures, answered.Erasures...)
	}
	for _, item := range skipped {
		printf(cli.Err, "case %s: %s - not sent; name its subject from the incident record and send it with --from\n",
			item.id, item.outcome)
	}
	return cli.Emit(all, reappliedTable(all, skipped))
}

// erasureFile reads a JSON array of cases, or an object carrying them under "erasures".
func erasureFile(path string) ([]openapi.ErasureCaseRecord, error) {
	raw, err := os.ReadFile(path) //nolint:gosec // the operator names the file on the command line
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	var records []openapi.ErasureCaseRecord
	if err := json.Unmarshal(raw, &records); err == nil {
		return records, nil
	}
	var wrapped struct {
		Erasures []openapi.ErasureCaseRecord `json:"erasures"`
	}
	if err := json.Unmarshal(raw, &wrapped); err != nil {
		return nil, fmt.Errorf("%s holds no erasure cases: %w", path, err)
	}
	return wrapped.Erasures, nil
}

// erasureRecordsOf turns the cluster's cases into the operation's records, setting aside those
// whose subject could not be named.
func erasureRecordsOf(read []postgres.RecoveredErasure) ([]openapi.ErasureCaseRecord, []unsent, error) {
	records := make([]openapi.ErasureCaseRecord, 0, len(read))
	var skipped []unsent
	for _, recovered := range read {
		if recovered.Unresolved != "" {
			skipped = append(skipped, unsent{id: recovered.Case.ID.String(), outcome: recovered.Unresolved})
			continue
		}
		record, err := erasureRecordOf(recovered.Case)
		if err != nil {
			return nil, nil, err
		}
		records = append(records, record)
	}
	return records, skipped, nil
}

// erasureRecordOf is one case as the operation takes it.
func erasureRecordOf(request domain.Request) (openapi.ErasureCaseRecord, error) {
	optionalID := func(id shared.ID) (*openapitypes.UUID, error) {
		if id.IsZero() {
			return nil, nil
		}
		parsed, err := uuid.Parse(id.String())
		if err != nil {
			return nil, fmt.Errorf("case %s names an unreadable identifier: %w", request.ID, err)
		}
		return &parsed, nil
	}
	optionalText := func(text string) *string {
		if text == "" {
			return nil
		}
		return &text
	}
	var record openapi.ErasureCaseRecord
	var err error
	if record.Id, err = uuid.Parse(request.ID.String()); err != nil {
		return record, fmt.Errorf("a case's identifier: %w", err)
	}
	if record.SubjectAccountId, err = optionalID(request.SubjectAccountID); err != nil {
		return record, err
	}
	if record.HandledBy, err = optionalID(request.HandledBy); err != nil {
		return record, err
	}
	record.ErasureMode = openapi.ErasureMode(request.ErasureMode)
	record.ReceivedAt = request.ReceivedAt.UTC()
	record.DueAt = request.DueAt.UTC()
	record.CompletedAt = request.CompletedAt.UTC()
	record.SubjectEmail = optionalText(request.SubjectEmail)
	record.Notes = optionalText(request.Notes)
	if !request.OriginalDueAt.IsZero() {
		original := request.OriginalDueAt.UTC()
		record.OriginalDueAt = &original
	}
	if request.ExtensionReason != "" {
		reason := openapi.DataSubjectRequestExtensionReason(request.ExtensionReason)
		record.ExtensionReason = &reason
	}
	if !request.InformedOn.IsZero() {
		record.InformedOn = &openapitypes.Date{Time: request.InformedOn.Midnight()}
	}
	return record, nil
}

// reappliedTable says per case what happened, the cases set aside included.
func reappliedTable(result openapi.ErasureReapplyResult, skipped []unsent) Table {
	rows := make([][]string, 0, len(result.Erasures)+len(skipped))
	for _, erasure := range result.Erasures {
		code := ""
		if erasure.Code != nil {
			code = *erasure.Code
		}
		rows = append(rows, []string{
			erasure.Id.String(), string(erasure.Outcome), strconv.FormatBool(erasure.Kept), code,
		})
	}
	for _, item := range skipped {
		rows = append(rows, []string{item.id, item.outcome, "false", ""})
	}
	return Table{Columns: []string{"id", "outcome", "kept by a hold", "code"}, Rows: rows}
}
