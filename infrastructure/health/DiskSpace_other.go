// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build !linux && !darwin

package health

import "errors"

// freeSpace cannot answer on this platform; the volume then says nothing.
func freeSpace(string) (free, total uint64, err error) {
	return 0, 0, errors.New("free space is measured on linux and darwin only")
}
