// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The installation's use per workspace and day (`GET /admin/usage`), a page of workspaces at a
 * time. Offered only while the operator has switched metering on; the refusal that says it is off
 * is told apart here, so the screen can say how to switch it on rather than show a failure.
 */

import type { ResourceState, UsagePage, WorkspaceUsage } from '@hubtask/sync-engine';

import { engine } from './engine.ts';
import { usageRange } from '../instance/usage.ts';

const USAGE = '/admin/usage';
const METERING_OFF = 'admin.metering_off';

function pathOf(cursor?: string): string {
  const { from, to } = usageRange();
  const query = new URLSearchParams({ from, to });
  if (cursor) query.set('cursor', cursor);
  return `${USAGE}?${query.toString()}`;
}

class Usage {
  #first = $state<ResourceState<UsagePage>>({ status: 'idle' });
  #more = $state<readonly WorkspaceUsage[]>([]);
  #cursor = $state<string | undefined>(undefined);

  get state(): ResourceState<UsagePage> {
    return this.#first;
  }

  /** Whether the server refused because metering is off - a choice, not a failure. */
  get isMeteringOff(): boolean {
    const read = this.#first;
    return read.status === 'failed' && (read.error.detailCode === METERING_OFF || read.error.code === METERING_OFF);
  }

  get rows(): readonly WorkspaceUsage[] {
    const first = this.#first.status === 'ready' ? this.#first.data.data : [];
    return [...first, ...this.#more];
  }

  get moreAfter(): string | undefined {
    return this.#cursor;
  }

  /** The first page, read and kept fresh. **From `untrack`.** */
  open(): () => void {
    return engine.subscribe<UsagePage>({ path: pathOf() }, (next) => {
      this.#first = next;
      if (next.status === 'ready') {
        this.#more = [];
        this.#cursor = next.data.page?.next_cursor ?? undefined;
      }
    });
  }

  /** The next page, appended. Cursor pagination, never page numbers. */
  async more(): Promise<void> {
    const cursor = this.#cursor;
    if (!cursor) return;
    const next = await engine.refresh<UsagePage>({ path: pathOf(cursor) });
    if (next.status === 'failed') throw next.error;
    if (next.status !== 'ready') return;
    this.#more = [...this.#more, ...next.data.data];
    this.#cursor = next.data.page?.next_cursor ?? undefined;
  }
}

export const usage = new Usage();
