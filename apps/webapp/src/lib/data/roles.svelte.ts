// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The permission catalogue, the shipped and the workspace's own roles and their grants, and what a
 * person holds at a place (UC-ID-26). Read where the administration shows them; every write is
 * the server's to allow, and it refuses a permission above the writer's own.
 */

import type {
  AccessExplanation, CustomRole, CustomRoleGrant, CustomRoleGrantCreate, ResourceState, RoleCatalogue,
} from '@hubtask/sync-engine';

import { engine } from './engine.ts';
import { etagFor } from './etag.ts';

export * from './roles.ts';

const PATH = '/roles';
const GRANTS = '/role-grants';
/** A custom role's grant is a membership the decision reads, so a write here changes what may be done. */
const TOUCHES = [PATH, '/access/explain', '/memberships'];

/** A place: the workspace, a hub or a collection. */
export interface Place {
  readonly scopeType: 'TENANT' | 'HUB' | 'COLLECTION';
  readonly scopeId?: string;
}

class Roles {
  #state = $state<ResourceState<RoleCatalogue> | undefined>(undefined);

  get state(): ResourceState<RoleCatalogue> | undefined {
    return this.#state;
  }

  /** The catalogue and the roles, undefined until read. */
  get catalogue(): RoleCatalogue | undefined {
    return this.#state?.status === 'ready' ? this.#state.data : undefined;
  }

  /** Starts the read. **From `untrack`**, for the reason the other stores record. */
  open(): () => void {
    return engine.subscribe<RoleCatalogue>({ path: PATH }, (next) => {
      this.#state = next;
    });
  }

  create(key: string, name: string, permissions: readonly string[]): Promise<CustomRole> {
    return engine.mutate<CustomRole>('POST', PATH, { key, name, permissions }, {
      idempotencyKey: crypto.randomUUID(), invalidates: TOUCHES,
    });
  }

  change(role: Pick<CustomRole, 'key' | 'version'>, name: string, permissions: readonly string[]): Promise<CustomRole> {
    return engine.mutate<CustomRole>('PATCH', `${PATH}/${encodeURIComponent(role.key)}`, { name, permissions }, {
      ifMatch: etagFor(role.version), invalidates: TOUCHES,
    });
  }

  async remove(key: string): Promise<void> {
    await engine.mutate<void>('DELETE', `${PATH}/${encodeURIComponent(key)}`, undefined, { invalidates: TOUCHES });
  }

  grant(body: CustomRoleGrantCreate): Promise<CustomRoleGrant> {
    return engine.mutate<CustomRoleGrant>('POST', GRANTS, body, {
      idempotencyKey: crypto.randomUUID(), invalidates: TOUCHES,
    });
  }

  async revoke(grantId: string): Promise<void> {
    await engine.mutate<void>('DELETE', `${GRANTS}/${grantId}`, undefined, { invalidates: TOUCHES });
  }

  /**
   * What a person holds at a place, and - given a permission - whether and through which role.
   * Omitting the person asks about the reader. Read afresh each time: it answers a question.
   */
  async explain(place: Place, accountId?: string, permission?: string): Promise<AccessExplanation | undefined> {
    const query = new URLSearchParams({ scope_type: place.scopeType });
    if (place.scopeId) query.set('scope_id', place.scopeId);
    if (accountId) query.set('account_id', accountId);
    if (permission) query.set('permission', permission);
    const state = await engine.refresh<AccessExplanation>({ path: `/access/explain?${query.toString()}` });
    if (state.status === 'failed') throw state.error;
    return state.status === 'ready' ? state.data : undefined;
  }
}

export const roles = new Roles();
