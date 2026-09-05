// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * How far this client is to be relied on, in one place.
 *
 * ADR-0035 §2's answer to "can I rely on this yet?" is a stage rather than a second version
 * number, and the application says it about itself while it is not `stable`. One constant, because
 * convergence changes it by changing this line — a stage repeated in a banner, a footer and a
 * release note is three statements that can disagree.
 *
 * It is deliberately **not** read from `/meta/capabilities`. ADR-0035 §2 says why: `web_ui` there
 * is a runtime fact — is an interface being served? — and a maturity stage is a statement about a
 * release. Putting it in the manifest would make a product adjective part of the API contract,
 * which is exactly the kind of thing that then cannot be removed.
 */

export type MaturityStage = 'experimental' | 'preview' | 'stable';

/**
 * `preview` since F2, and the reason is the evidence rather than the calendar.
 *
 * ADR-0035 §2 draws the line at a promise: `experimental` says nothing here is a commitment,
 * `preview` says what is shown is meant to stay and is usable for real work, with the gaps against
 * the capability matrix expected and listed. A day's work was done in this application against
 * this repository's own backlog and written down in `docs/evidence/R-08-2026-09-04.md`: a
 * collection for a milestone, its tasks, one broken into work packages and an activity, labelled,
 * ordered, completed, archived, found by a word in its notes, and its history read.
 *
 * That pass found four writes that existed in the stores and reached no screen — a workspace could
 * be worked in and not *started*, which is what kept the stage where F1 put it. All four are built
 * (#359, #360, #361, #362), so the last argument for `experimental` is gone: nothing in the pass
 * suggested anything here is going to be taken away, and gaps are gaps rather than churn.
 */
export const MATURITY: MaturityStage = 'preview';

/**
 * Whether the application owes the reader a notice about itself.
 *
 * A function taking the stage rather than a constant, for a reason the compiler makes plain: a
 * `const` compared against a literal it cannot be is a comparison TypeScript rejects as pointless,
 * and it is right - the answer is only interesting for a stage that is not the one compiled in.
 * Taking the stage as an argument keeps the rule checkable at every value it can hold.
 */
export function shouldAnnounce(stage: MaturityStage = MATURITY): boolean {
  return stage !== 'stable';
}
