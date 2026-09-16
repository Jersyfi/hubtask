// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package importer

import (
	"context"
	"strings"
	"testing"
	"time"

	backup "github.com/Jersyfi/hubtask/core/domain/model/backup"
	domain "github.com/Jersyfi/hubtask/core/domain/model/importer"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// The port carries no logic - the doubles prove both interfaces can be implemented by a fake,
// which is what the use case tests depend on.
type runsDouble struct{}

func (runsDouble) Insert(context.Context, domain.Run) error                  { return nil }
func (runsDouble) Find(context.Context, shared.ID) (domain.Run, error)       { return domain.Run{}, nil }
func (runsDouble) Claim(context.Context, shared.ID, time.Time) (bool, error) { return true, nil }
func (runsDouble) RecordProgress(context.Context, shared.ID, backup.Report, map[string]int) error {
	return nil
}
func (runsDouble) Finish(context.Context, domain.Outcome) error { return nil }

type converterDouble struct{}

func (converterDouble) Kind() domain.Kind { return domain.KindCSV }
func (converterDouble) Convert(context.Context, Source) (Result, error) {
	return Result{Refused: []domain.Refusal{{Row: 2, Code: domain.CodeRowTitleMissing}}}, nil
}

func TestThePortsCanBeImplementedByADouble(t *testing.T) {
	var runs Runs = runsDouble{}
	if ok, err := runs.Claim(context.Background(), shared.ID(""), time.Time{}); !ok || err != nil {
		t.Fatalf("claim: %v %v", ok, err)
	}
	var converter Converter = converterDouble{}
	result, err := converter.Convert(context.Background(), Source{Content: strings.NewReader("title\n"), Digest: "abc"})
	if err != nil || len(result.Refused) != 1 || converter.Kind() != domain.KindCSV {
		t.Fatalf("convert: %+v %v", result, err)
	}
}
