// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package filestore

import (
	"context"
	"reflect"
	"testing"
)

// The tenant travels on the context into the transaction wrapper (rule 3) together with the
// deadline (rule 7), and a refused or failed write must be able to say so. A method without a
// leading context or a trailing error would be an adapter that can do neither.
func TestEveryPortMethodTakesAContextAndCanFail(t *testing.T) {
	ctxType := reflect.TypeFor[context.Context]()
	errType := reflect.TypeFor[error]()
	for _, port := range []reflect.Type{
		reflect.TypeFor[Store](),
	} {
		for i := range port.NumMethod() {
			m := port.Method(i)
			if m.Type.NumIn() == 0 || m.Type.In(0) != ctxType {
				t.Errorf("%s.%s does not take a context.Context first", port.Name(), m.Name)
			}
			if m.Type.NumOut() == 0 || m.Type.Out(m.Type.NumOut()-1) != errType {
				t.Errorf("%s.%s does not return an error last", port.Name(), m.Name)
			}
		}
	}
}
