// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The address a calling system posts to for an `INBOUND_WEBHOOK` rule (issue 775): the route
 * under the API at the origin that serves it, with the token as the whole credential
 * (`automation.md` §1.1). Composed here for `caldavAddressOf`'s reason - the API and the
 * interface come from one origin (ADR-0028), so the screen knows where the route is - and as a
 * pure function, so that the shape is held by a test rather than by a walk.
 */
export function inboundAddressOf(origin: string, token: string): string {
  return `${origin.replace(/\/+$/, '')}/api/v1/automation/inbound/${token}`;
}
