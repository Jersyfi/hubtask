// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The approvals' paths and writes (UC-SVC-08 to 10): the reader's own list, an entry's approvals
 * and their trail, a decision, and the reader's delegation while away. A decision invalidates the
 * list, the entry's approvals and the entry itself, which the last yes moves.
 */

import type { Approval, ApprovalDelegation } from '@hubtask/sync-engine';

import { engine } from './engine.ts';

export const myApprovalsPath = '/approvals';
export const entryApprovalsPath = (itemId: string) => `/items/${itemId}/approvals`;
export const delegationPath = '/approval-delegation';

export type Verdict = 'APPROVE' | 'DECLINE';

class Approvals {
  decide(approval: Pick<Approval, 'id' | 'item_id'>, verdict: Verdict, reason: string): Promise<Approval> {
    const body = reason.trim() === '' ? { verdict } : { verdict, reason: reason.trim() };
    return engine.mutate<Approval>('POST', `/approvals/${approval.id}:decide`, body, {
      idempotencyKey: crypto.randomUUID(),
      invalidates: [myApprovalsPath, entryApprovalsPath(approval.item_id), `/items/${approval.item_id}$`],
    });
  }

  delegate(delegation: ApprovalDelegation): Promise<ApprovalDelegation> {
    return engine.mutate<ApprovalDelegation>('PUT', delegationPath, delegation, { invalidates: [delegationPath] });
  }

  undelegate(): Promise<unknown> {
    return engine.mutate('DELETE', delegationPath, undefined, { invalidates: [delegationPath] });
  }
}

export const approvals = new Approvals();
