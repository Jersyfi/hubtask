// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The workspaces screen's narrowing as `POST /admin/tenants:query` takes it (TenantQuery): each
 * field optional, all of them together, a page at a time.
 */

/** One page of the workspaces screen: enough to scan, the rest a search or a "more" away. */
export const TENANT_PAGE = 50;

/** What the workspaces screen narrows by: each optional, all of them together. */
export interface WorkspaceFilter {
  readonly slug?: string;
  readonly reference?: string;
  readonly plan?: string;
  readonly status?: 'ACTIVE' | 'SUSPENDED' | 'PENDING_DELETION';
}

/** The query document for a narrowing; an empty field narrows nothing. */
export function workspaceQuery(filter: WorkspaceFilter, size = TENANT_PAGE): Record<string, unknown> {
  const slug = filter.slug?.trim().toLowerCase();
  const reference = filter.reference?.trim();
  return {
    ...(slug ? { slug } : {}),
    ...(reference ? { external_reference: reference } : {}),
    ...(filter.plan === '' || filter.plan === undefined ? {} : filter.plan === '-' ? { without_plan: true } : { plan: filter.plan }),
    ...(filter.status ? { status: filter.status } : {}),
    size,
  };
}
