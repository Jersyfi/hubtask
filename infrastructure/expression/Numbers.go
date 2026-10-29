// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package expression

import (
	"cel.dev/cel-go/cel"
	celast "cel.dev/cel-go/common/ast"
	"cel.dev/cel-go/common/types"
)

// wholeNumbersAsDoubles rewrites a formula's whole-number literals into doubles. CEL keeps int and
// double apart and a number field always reaches a formula as a double, so `hours * 2` failed at
// evaluation and had to be written `hours * 2.0`. A literal, not an operator overload: the
// library's division is a singleton that admits no second signature, and a rewritten literal
// leaves the arithmetic CEL's own. Only a formula is rewritten - a condition's `size(x) > 2`
// compares two ints and stays one.
type wholeNumbersAsDoubles struct{}

func (wholeNumbersAsDoubles) Optimize(ctx *cel.OptimizerContext, a *celast.AST) *celast.AST {
	literals := celast.MatchDescendants(celast.NavigateAST(a), func(e celast.NavigableExpr) bool {
		return e.Kind() == celast.LiteralKind && e.AsLiteral().Type() == types.IntType
	})
	for _, literal := range literals {
		whole := literal.AsLiteral().(types.Int)
		ctx.UpdateExpr(literal, ctx.NewLiteral(types.Double(whole)))
	}
	return a
}

// asFormula answers the checked formula with its whole numbers as doubles, checked again.
func asFormula(env *cel.Env, checked *cel.Ast) (*cel.Ast, error) {
	optimizer, err := cel.NewStaticOptimizer(wholeNumbersAsDoubles{})
	if err != nil {
		return nil, err
	}
	optimized, issues := optimizer.Optimize(env, checked)
	if issues != nil && issues.Err() != nil {
		return nil, issues.Err()
	}
	return optimized, nil
}
