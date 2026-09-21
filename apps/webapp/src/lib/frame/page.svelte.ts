// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * What the current page tells the frame about itself: its title, for the bar on a phone.
 *
 * Below `medium` the app bar carries the page's title in place of the wordmark, and the page head
 * asks for its `h1` to be read rather than drawn (`PageHeader`'s `isTitleInBar`), so a screen
 * keeps one heading and the reader is told the title once (ADR-0061 decision 2). The view that
 * hides its heading is the one that knows the title, so the view sets it here and clears it when
 * it leaves; a frame that derived titles from routes would be a second list of what each screen
 * is called. A view that sets nothing keeps its heading and the bar keeps the wordmark.
 */

class Page {
  #title = $state<string | undefined>(undefined);

  get title(): string | undefined {
    return this.#title;
  }

  /** Sets the title for as long as the caller's effect lives; the cleanup clears it. */
  entitle(title: string | undefined): () => void {
    this.#title = title;
    return () => {
      if (this.#title === title) this.#title = undefined;
    };
  }
}

export const page = new Page();
