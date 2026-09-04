// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * What the application says out loud when something changed where nobody was looking.
 *
 * A rank change is the case that needs it and the reason this exists. Pressing "move down" moves a
 * row that the reader may not be able to see, and a screen reader announces **nothing** for it:
 * the focused control did not change, its name did not change, and the list quietly rearranged
 * itself. WCAG 2.2 SC 2.5.7's single-pointer alternative is only an alternative if it tells the
 * reader what it did.
 *
 * **One region for the whole application, in the frame.** Two live regions compete — a screen
 * reader is watching both and reads whichever changed, in an order nobody chose — and a region
 * created at the moment it has something to say is a region nothing was watching, which is the
 * defect `LoadMore` records in its own markup.
 *
 * `polite` rather than `assertive`: the reader asked for this, so it waits for the next pause
 * instead of interrupting them mid-sentence.
 */
class Announcer {
  #message = $state('');

  get message(): string {
    return this.#message;
  }

  /**
   * Says something. Resolved text (ADR-0011) — the caller has the parameters, so the caller calls
   * `t`.
   *
   * The same sentence twice is a change a live region cannot see: the text is identical, so
   * nothing was updated and nothing is read. Moving a row down twice is exactly that case, so the
   * region is cleared first and the sentence set in a microtask — which is one render apart, and
   * enough for the observer to notice both.
   */
  say(message: string): void {
    this.#message = '';
    queueMicrotask(() => {
      this.#message = message;
    });
  }
}

export const announcer = new Announcer();
