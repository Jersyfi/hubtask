// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * One form's submit, asked what is missing (UC-WRK-32 check 3): `missing.ts`'s answer, and focus
 * moved to the first marked field once the marks are drawn. The administration's forms each did
 * this by hand or returned in silence; this is the one way they do it now.
 *
 * Usage, in a form's `onsubmit`:
 *
 *     missing = marked(event, [{ kind: 'field', key: 'name', code: 'app.forms.required', isFilled: name.trim() !== '' }]);
 *     if (!isComplete(missing)) return;
 */

import { tick } from 'svelte';

import { check, focusFirst, isComplete, type Missing, type Requirement } from './missing.ts';

/** What is missing; where something is, focus goes to the first marked field of the form. */
export function marked(event: SubmitEvent, requirements: readonly Requirement[]): Missing {
  const missing = check(requirements);
  if (!isComplete(missing)) {
    // Kept before the tick: an event's current target is gone once it has been dispatched.
    const form = event.currentTarget as HTMLFormElement | null;
    void tick().then(() => focusFirst(form));
  }
  return missing;
}
