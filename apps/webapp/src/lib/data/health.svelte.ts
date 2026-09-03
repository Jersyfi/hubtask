// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The installation's deep self-diagnosis, read only by somebody who may read it.
 *
 * `/meta/health` needs `admin:read` (`observability-reliability.md` §5), and that constraint
 * shapes this module more than anything else does. An ordinary member is not shown a report they
 * are not entitled to, is not shown an error about being refused one, and does not send a request
 * that will be refused: the read is not attempted at all without a bearer, and a `401` or `403`
 * ends it quietly rather than becoming a message on the screen.
 *
 * F1's header decision is the other half of that: **no second, unauthenticated health surface**.
 * A member learns of a degradation when an operation degrades, which is where the message belongs
 * anyway - "media is unavailable" is useful beside an upload and noise on a dashboard.
 */

import type { HealthReport, ResourceState } from '@hubtask/sync-engine';

import { platform } from '../platform/index.ts';
import { engine } from './engine.ts';

const PATH = '/meta/health';

/** One feature the installation is not serving fully, with the code that says why. */
export interface Degradation {
  readonly feature: string;
  readonly reasonCode: string;
}

class Health {
  #state = $state<ResourceState<HealthReport>>({ status: 'idle' });

  get state(): ResourceState<HealthReport> {
    return this.#state;
  }

  get report(): HealthReport | undefined {
    return this.#state.status === 'ready' ? this.#state.data : undefined;
  }

  /**
   * Whether the report says something is wrong. `disabled` is not: it means this installation does
   * not run the deep checks, which is a configuration rather than a fault.
   */
  get isTroubled(): boolean {
    const status = this.report?.status;
    return status === 'degraded' || status === 'down';
  }

  get isDown(): boolean {
    return this.report?.status === 'down';
  }

  /** What is degraded, as feature plus the message code that says why (`DegradedFeature`). */
  get degradations(): Degradation[] {
    return (this.report?.degraded_features ?? [])
      .filter((entry): entry is { feature: string; reason_code: string; since: string } =>
        typeof entry?.feature === 'string' && typeof entry?.reason_code === 'string',
      )
      .map((entry) => ({ feature: entry.feature, reasonCode: entry.reason_code }));
  }

  /**
   * Starts the read, or does not.
   *
   * Returns the stop function either way, so a caller has one shape to handle. Without a bearer
   * there is nobody to be entitled, and the request is not made - `platform.bearer()` is the seam
   * that knows, and it answers `undefined` until F1-11 puts a token behind it.
   */
  start(): () => void {
    if (platform.bearer() === undefined) return () => {};

    return engine.subscribe<HealthReport>({ path: PATH }, (next) => {
      // A refusal is an answer about *this reader*, not about the installation. It leaves the
      // state where it was, so nothing is drawn and nothing is said.
      if (next.status === 'failed' && (next.error.status === 401 || next.error.status === 403)) {
        this.#state = { status: 'idle' };
        return;
      }
      this.#state = next;
    });
  }
}

export const health = new Health();
