// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package usecase

import "testing"

// An answer misses its declaration where a required member is absent or null, or a declared
// member is of another kind; members the declaration does not name are allowed.
func TestAnAnswerIsHeldToItsDeclaredOutput(t *testing.T) {
	d := Descriptor{Output: []Field{
		{Name: "hash", Kind: KindString, Required: true},
		{Name: "next", Kind: KindString},
		{Name: "rows", Kind: KindList, Required: true},
		{Name: "count", Kind: KindInt, Required: true},
		{Name: "flags", Kind: KindObject},
	}}
	cases := map[string]struct {
		out    Output
		missed string
	}{
		"whole":            {out: Output{"hash": "h", "next": nil, "rows": []Output{}, "count": 2, "extra": true}},
		"required absent":  {out: Output{"rows": []any{}, "count": 1}, missed: "hash"},
		"required null":    {out: Output{"hash": nil, "rows": []any{}, "count": 1}, missed: "hash"},
		"optional of kind": {out: Output{"hash": "h", "next": 3, "rows": []any{}, "count": 1}, missed: "next"},
		"list of a kind":   {out: Output{"hash": "h", "rows": "no", "count": 1}, missed: "rows"},
		"integer":          {out: Output{"hash": "h", "rows": []string{}, "count": "1"}, missed: "count"},
		"object":           {out: Output{"hash": "h", "rows": []string{}, "count": int64(1), "flags": []any{}}, missed: "flags"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			member, missed := d.OutputMismatch(c.out)
			if member != c.missed || missed != (c.missed != "") {
				t.Fatalf("missed %q (%t), want %q", member, missed, c.missed)
			}
		})
	}
}
