<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // Code, shown as code: a `<pre>` that scrolls inside its own box, in the mono face, with the
  // language named beside it and, where the caller asks for one, a control that copies it.
  //
  // **Still no highlighter.** Colouring tokens needs a grammar per language, and every grammar in
  // circulation is a dependency - a supply chain decision (security.md §11) for a brochure that ships
  // no script at all (ADR-0030). What the market actually built on top of its highlighters over
  // the last few years is not more colour; it is **the line**. Shiki's transformers are line
  // numbers, a marked line, an added and a removed line, a focused line. Every one of those is a
  // statement about a row of text, none of them needs to know what a keyword is, and all of them
  // work with no script at all. So that is what this block learned, and the colour stays out.
  //
  // ## The three things a line gets
  //
  // **A number that cannot be copied.** The gutter is `aria-hidden` and `user-select: none`, so a
  // reader who selects the block and copies it gets the code and not `1 2 3 4` down the left. It
  // is the detail every documentation site gets wrong once.
  //
  // **A mark, where the caller marks one.** `added` and `removed` carry a `+` and a `−` in the
  // gutter as well as a tint, because a diff told apart by colour alone is a diff half the readers
  // cannot read (design-system.md rule 3). `marked` is the line the prose is about. `faded` is
  // Shiki's focus inverted - the caller names what matters and the rest recedes, rather than
  // naming everything that does not.
  //
  // **A prompt that is not part of the command.** In a shell block a `$ ` at the start of a line
  // moves into the gutter: it is punctuation telling the reader "type this", and a reader who
  // copies it pastes an error. The command is what ends up on the clipboard, which is also what
  // `copy` hands over.
  //
  // The copy control is the caller's choice, as `OneTimeSecret`'s is: a page that renders without
  // JavaScript passes no `copyLabel` and gets no button that could not work. Where it is asked
  // for, it is offered only where `navigator.clipboard` exists.

  import Button from './Button.svelte';
  import { canCopy } from './secret.ts';

  /** What a line is, beyond being a line. One statement each, never two at once. */
  export type LineKind = 'added' | 'removed' | 'marked' | 'faded';

  interface Props {
    code: string;
    /** What the block is, for the accessibility tree: "Request", "Example response". */
    label: string;
    /** The language, drawn as a small label. `bash`, `json`, `http`. Absent means unlabelled. */
    language?: string;
    /**
     * Where the code lives, drawn in the mono face beside the label: `db/queries/items.sql`. A
     * path, never a sentence - it is not translated and it is not phrased.
     */
    fileName?: string;
    /** Whether the lines are numbered. */
    hasLineNumbers?: boolean;
    /**
     * What each line is, by one-based line number. A line nobody names is an ordinary line.
     * Resolved by the caller, because which line a paragraph is about is the prose's question.
     */
    lines?: Readonly<Record<number, LineKind>>;
    /**
     * A long line wraps with a hanging indent instead of scrolling. Off by default: a `curl` that
     * wraps has lost where its arguments begin, and scrolling keeps the shape. On for prose-like
     * payloads a reader reads rather than copies.
     */
    isWrapped?: boolean;
    /**
     * `$ ` at the start of a line is a prompt, not a command. Only ever true for a shell block,
     * and never inferred from `language` - a `bash` block showing a script has no prompts, and
     * guessing would eat a legitimate `$`.
     */
    hasPrompts?: boolean;
    /** Present means a copy control is offered where the browser can copy. */
    copyLabel?: string;
    /** Announced after a copy. Says that it was copied, never what was copied. */
    copiedLabel?: string;
  }

  const {
    code,
    label,
    language,
    fileName,
    hasLineNumbers = false,
    lines = {},
    isWrapped = false,
    hasPrompts = false,
    copyLabel,
    copiedLabel,
  }: Props = $props();

  /** One row of the block: its number, what it is, and the two halves of its text. */
  interface Line {
    readonly n: number;
    readonly kind: LineKind | undefined;
    /** The `$ ` a shell block moves into the gutter. Empty everywhere else. */
    readonly prompt: string;
    readonly text: string;
  }

  // A trailing newline is the file's, not a line: splitting without dropping it draws an empty
  // numbered row under every block that ends the way a file ends.
  const rows: readonly Line[] = $derived(
    code.replace(/\n$/, '').split('\n').map((raw, index) => {
      const isPrompt = hasPrompts && /^\s*\$ /.test(raw);
      return {
        n: index + 1,
        kind: lines[index + 1],
        prompt: isPrompt ? '$' : '',
        text: isPrompt ? raw.replace(/^(\s*)\$ /, '$1') : raw,
      };
    }),
  );

  /** What the clipboard gets: the code, with the prompts already gone. */
  const copyable = $derived(rows.map((row) => row.text).join('\n'));

  let hasCopied = $state(false);
  const hasClipboard = canCopy(globalThis.navigator?.clipboard);
  const isCopyable = $derived(copyLabel !== undefined && hasClipboard);

  async function copy(): Promise<void> {
    try {
      await globalThis.navigator.clipboard.writeText(copyable);
      hasCopied = true;
    } catch {
      hasCopied = false;
    }
  }
</script>

<figure class="block">
  <figcaption class="head">
    <span class="label">{label}</span>
    {#if fileName}<span class="file">{fileName}</span>{/if}
    {#if language}<span class="language">{language}</span>{/if}
    {#if isCopyable}
      <span class="copy">
        <Button tone="secondary" size="sm" icon="copy" onclick={() => void copy()}>{copyLabel}</Button>
      </span>
    {/if}
  </figcaption>
  <!-- Focusable for the same reason `Table`'s scroll region is: a wide line is otherwise out of
       reach without a pointer (WCAG 2.1.1). The region carries the figure's name.

       The lines are stacked boxes rather than a grid: a grid would lay the gutter and the text
       out in two columns, and a selection dragged across three lines would come out as three
       numbers followed by three lines of code. Here the gutter sits inside its line and is
       `user-select: none`, so a selection across the block yields the code and nothing else -
       which is also exactly what the copy control hands over. -->
  <!-- svelte-ignore a11y_no_noninteractive_tabindex -->
  <pre
    class="code"
    class:is-wrapped={isWrapped}
    class:has-numbers={hasLineNumbers}
    tabindex="0"
    role="region"
    aria-label={label}><code
      >{#each rows as row (row.n)}<span
          class="line"
          data-kind={row.kind}
          ><span class="gutter" aria-hidden="true"
            >{#if hasLineNumbers}<span class="n">{row.n}</span>{/if}<span class="sign"
              >{row.kind === 'added' ? '+' : row.kind === 'removed' ? '−' : row.prompt}</span
            ></span
          ><span class="text">{row.text}</span></span
        >{/each}</code
    ></pre>
  <span class="announce" role="status" aria-live="polite">{hasCopied && copiedLabel ? copiedLabel : ''}</span>
</figure>

<style>
  .block {
    margin: 0;
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-md);
    background: var(--bg-surface-sunken);
    color: var(--text-primary);
    /* Rule 4: as wide as it is given; the line scrolls, the block does not grow. */
    max-inline-size: 100%;
  }

  .head {
    display: flex;
    align-items: center;
    gap: var(--sp-100);
    padding: var(--sp-050) var(--sp-150);
    border-block-end: var(--bw-hairline) solid var(--border-subtle);
    font-size: var(--fs-075);
    color: var(--text-secondary);
  }

  .label { font-weight: var(--fw-medium); }

  /* A path, in the face it is a path in. `ltr` because a file path runs one way whatever the page
     does, the same reason the code below does. */
  .file {
    direction: ltr;
    color: var(--text-subtle);
    font-family: var(--font-mono);
  }

  .language { font-family: var(--font-mono); color: var(--text-subtle); }
  .copy { margin-inline-start: auto; }

  .code {
    display: block;
    margin: 0;
    padding-block: var(--sp-150);
    overflow-x: auto;
    font-family: var(--font-mono);
    font-size: var(--fs-075);
    line-height: var(--lh-normal);
    /* A `<pre>` keeps the author's line breaks; `text-align: start` keeps a code sample from
       being mirrored under `dir="rtl"`, because code runs one way whatever the page does. */
    direction: ltr;
    text-align: start;
    tab-size: 2;
  }

  /* The lines stack, so the element holding them is a block. A `<code>` left inline with
     block-level children would have the browser inventing anonymous boxes around each one. */
  .code code { display: block; }

  .code:focus-visible {
    outline: var(--bw-ring) solid var(--focus-ring);
    outline-offset: calc(var(--sp-025) * -1);
  }

  /* Each line is its own band, so a tint reaches the full width of the block rather than stopping
     at the end of the text. `min-inline-size: 100%` is what makes it reach across a block that is
     scrolled sideways, too - a marked line that ended at the fold would be marked for the reader
     who had not scrolled and unmarked for the one who had. */
  .line {
    display: flex;
    min-inline-size: 100%;
    inline-size: max-content;
    padding-inline-end: var(--sp-150);
  }

  /* The gutter: a number, and a sign or a prompt. `aria-hidden` and unselectable, so copying the
     block copies the code and not the furniture. */
  .gutter {
    display: flex;
    flex: none;
    gap: var(--sp-050);
    justify-content: flex-end;
    padding-inline: var(--sp-150) var(--sp-100);
    user-select: none;
    -webkit-user-select: none;
    color: var(--text-subtle);
  }

  /* The number column is as wide as four digits whether or not there are four, so the code does
     not step sideways at line 100. `ch` on the mono face is exactly one digit. */
  .n { min-inline-size: 4ch; text-align: end; } /* design-system-lint-ignore: `ch` is the font's own digit width, which is the point; no token can express it. */

  /* Always present and usually empty, so a `+` arriving on one line does not shove that line's
     text a character to the right of every other line's. */
  .sign { min-inline-size: 1ch; text-align: center; } /* design-system-lint-ignore: as above. */

  .text { white-space: pre; }

  /* A long line wraps under its own first column rather than under the gutter, so the wrapped
     remainder reads as a continuation and not as a new line. */
  .code.is-wrapped .line { inline-size: auto; }
  .code.is-wrapped .text { white-space: pre-wrap; overflow-wrap: anywhere; }

  /* Without numbers the gutter still holds the sign, so a diff and a plain block have their text
     on the same left edge and two blocks under one another do not disagree. */
  .code:not(.has-numbers) .n { display: none; }

  /* What a line is.

     Each of the three is a tint **and** a mark: the `+` and the `−` are in the gutter above, and
     the marked line carries a rule down its edge. A diff told apart by colour alone is a diff
     half the readers cannot read (design-system.md rule 3), and that is before anybody prints it.
     The tints are the status surfaces the rest of the product already uses, so a green here is
     the same green as a success anywhere else. */
  .line[data-kind='added'] {
    background: var(--status-success-surface);
    box-shadow: inset var(--bw-thick) 0 0 0 var(--status-success-border);
  }
  .line[data-kind='added'] .gutter { color: var(--status-success-text); }

  .line[data-kind='removed'] {
    background: var(--status-danger-surface);
    box-shadow: inset var(--bw-thick) 0 0 0 var(--status-danger-border);
  }
  .line[data-kind='removed'] .gutter { color: var(--status-danger-text); }

  /* The marked line is **lifted out of** the block rather than tinted in it: `bg-surface` is one
     step up from the `bg-surface-sunken` the block sits at, and it is a step in both modes. The
     obvious choice was `accent-primary-subtle`, and it is invisible here - in light mode that
     token and `bg-surface-sunken` differ by two counts of one channel, because one is the blue
     ramp's palest step and the other is a neutral already tinted towards blue. The blue is still
     in the rule down the edge, which is the half that carries the meaning (rule 3). */
  .line[data-kind='marked'] {
    background: var(--bg-surface);
    box-shadow: inset var(--bw-thick) 0 0 0 var(--accent-primary);
  }

  /* The lines the prose is not about. Opacity rather than a paler colour, because the thing being
     said is "this is still here and is not the point" - and it is one value, so it does not need
     a pair measured against a surface. It stays above the contrast floor for body text at rest:
     the reader is meant to be able to read it, just not first. */
  .line[data-kind='faded'] { opacity: 0.55; }

  /* Announced, never drawn (the `Table` technique). */
  .announce {
    position: absolute;
    inline-size: var(--sp-025);
    block-size: var(--sp-025);
    margin: calc(var(--sp-025) * -1);
    overflow: hidden;
    clip-path: inset(50%);
    white-space: nowrap;
  }
</style>
