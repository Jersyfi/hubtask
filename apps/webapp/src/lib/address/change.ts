// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/** The workspace whose address changes, as the change-of-address dialog names it. */
export interface AddressSubject {
  readonly id: string;
  readonly name: string;
  readonly slug: string;
  readonly host?: string;
}

/** What a change of address, or its dry run, answers - the operator's and the owner's alike. */
export interface AddressAnswer {
  readonly from: string;
  readonly to?: string;
  readonly members_mailed: number;
  readonly members_told_at_sign_in: number;
}

/**
 * Where the browser goes once the owner changed the workspace's address: the new address's front
 * door, on the scheme and port it came by. The old address answers nothing from the moment of the
 * change and every session there ended with it, so staying would be a sign-in that leads nowhere.
 */
export function addressAfterChange(to: string, current: { readonly protocol: string; readonly port: string }): string {
  const port = current.port ? `:${current.port}` : '';
  return `${current.protocol}//${to}${port}/`;
}
