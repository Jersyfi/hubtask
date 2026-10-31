// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package importer

import (
	"context"
	"io"

	service "github.com/Jersyfi/hubtask/core/application/repository/importer"
	domain "github.com/Jersyfi/hubtask/core/domain/model/importer"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/shared/spreadsheet"
)

// XLSX is a workbook's first sheet read as CSV is: the header row, one entry per row, the same
// columns, mapping and refusals by row number. Dates arrive as ISO dates and numbers as stored,
// so nothing passes through a locale's format on the way.
type XLSX struct{}

var _ service.Converter = XLSX{}

func (XLSX) Kind() domain.Kind { return domain.KindXLSX }

func (XLSX) Convert(_ context.Context, source service.Source) (service.Result, error) {
	raw, err := io.ReadAll(io.LimitReader(source.Content, spreadsheet.MaxPartBytes+1))
	if err != nil {
		return service.Result{}, err
	}
	rows, err := spreadsheet.Rows(raw)
	if err != nil {
		return service.Result{}, shared.ErrValidation.WithDetail(domain.CodeFileNotKind)
	}
	if len(rows) == 0 {
		return service.Result{}, shared.ErrValidation.WithDetail(domain.CodeFileEmpty)
	}
	rest := rows[1:]
	return convertTable(source, rows[0], func() ([]string, error) {
		if len(rest) == 0 {
			return nil, io.EOF
		}
		row := rest[0]
		rest = rest[1:]
		return row, nil
	})
}
