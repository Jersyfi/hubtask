// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// RFC 9116's contact file, prerendered into dist/.well-known/ (security.md §14.2).

import { securityTxt } from '#lib/securityTxt.ts';

export const prerender = true;

export function GET(): Response {
  return new Response(securityTxt(new Date()), { headers: { 'content-type': 'text/plain; charset=utf-8' } });
}
