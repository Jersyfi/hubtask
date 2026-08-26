// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build linux

package backupstorage

import "syscall"

// blockSizeOf reads the field whose type is the reason this file exists: it is int64 on Linux and
// int32 on Darwin, and one expression covering both needs a conversion that is redundant on one
// of them. A function per platform says it once, in the open, rather than with a suppression.
func blockSizeOf(stat syscall.Statfs_t) int64 { return stat.Bsize }
