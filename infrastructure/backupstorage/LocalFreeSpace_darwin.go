// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build darwin

package backupstorage

import "syscall"

// blockSizeOf reads the field whose type is the reason this file exists. See the Linux twin.
func blockSizeOf(stat syscall.Statfs_t) int64 { return int64(stat.Bsize) }
