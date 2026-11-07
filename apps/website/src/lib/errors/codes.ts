// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The error codes the API answers with, one page each at /errors/<code>/ - where a problem
 * document's `docs` link points. The list is JSON rather than code so that the Go gate
 * (test/architecture/errorpages_test.go) can hold it equal to the codes the server makes.
 */
import list from './codes.json' with { type: 'json' };

export interface ErrorCode {
  code: string;
  status: number;
  title: string;
  meaning: string;
  action: string;
  retry: boolean;
}

export const codes: readonly ErrorCode[] = [...list.codes].sort(
  (a, b) => a.status - b.status || a.code.localeCompare(b.code),
);
