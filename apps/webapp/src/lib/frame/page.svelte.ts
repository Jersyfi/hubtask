// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * What the current page tells the frame about itself: its title, for the bar on a phone, and
 * whether it draws its own edges.
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
  #fills = $state(false);

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

  /**
   * Whether the page draws its own edges and takes the content region whole.
   *
   * Almost every screen is a document in the frame's padding, with the reading measure capped.
   * A canvas is not: the rule editor is a surface with its own head, its own hairlines and an
   * inspector against the far edge, and standing it inside the padding drew a slab of one colour
   * on a page of another - a box on a page, which is what issue 918 was. The page says so and
   * the frame gives it the room; the same answer would serve any later canvas, and there is no
   * route table in the frame to keep in step.
   *
   * **The room is a height as well as a width** (`milestone-F8.md` decision 31): a filled page
   * is one screen, so the frame bounds the region to the viewport and what scrolls is inside the
   * page - its canvas, its panel - rather than the page itself. Without that bound the editor
   * grew past the fold and its panel, as tall as the viewport but starting below the bar and the
   * notices, ended below the window with no way to reach it.
   */
  get fills(): boolean {
    return this.#fills;
  }

  /** Takes the content region for as long as the caller's effect lives. */
  fill(): () => void {
    this.#fills = true;
    return () => {
      this.#fills = false;
    };
  }
}

export const page = new Page();
