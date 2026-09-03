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

/** F1 and F2 are the first two client milestones, which is what `experimental` names. */
export const MATURITY: MaturityStage = 'experimental';

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
