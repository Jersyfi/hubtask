<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // One field for the whole question: the words and the narrowing together (ADR-0066 decision 2).
  //
  // The language it accepts is `data/searchquery.ts`, which is pure and tested beside itself; this
  // is the surface. It is the *same state* as the chips above it — one string, read and written by
  // both — so neither can be the stale one.
  //
  // The words and the narrowing are written in the same line, because they are one question: "the
  // tiles invoice, in the kitchen, still open" is a sentence somebody says in one breath, and
  // today it takes a field and three popovers. What makes that safe rather than clever is that
  // the line is **compiled** rather than sent: every token becomes a node in the grammar the
  // contract already documents, and a token this installation cannot answer is refused here by
  // name rather than turned into a request that comes back empty.
  //
  // **Nothing is hidden behind the syntax.** The tokens are drawn back out as chips under the
  // field, each removable; the ways to narrow are offered as you type; and what the line means is
  // shown as the request it sends. A language nobody can discover is a language nobody uses, which
  // is the one thing every product that has tried this got wrong at least once.

  import { Button, Stack } from '@hubtask/design-system/components';
  import { SearchField } from '@hubtask/design-system/components';

  import {
    KEYS,
    SORTS,
    compile,
    parse,
    toggle,
    write,
    type Compiled,
    type Context,
    type Key,
    type Parsed,
  } from '../data/searchquery.ts';
  import { t } from '../i18n/i18n.svelte.ts';

  interface Props {
    /** The whole line, words and tokens together. */
    value: string;
    /** What the keys resolve against: the installation's fields and this workspace's names. */
    context: Context;
    onchange: (line: string) => void;
    /** The compiled question, handed up so the screen can ask it. */
    oncompiled?: (compiled: Compiled) => void;
  }

  let { value, context, onchange, oncompiled }: Props = $props();

  const parsed = $derived<Parsed>(parse(value));
  const compiled = $derived<Compiled>(compile(parsed, context));

  $effect(() => oncompiled?.(compiled));

  /** The answers each key offers, for the list under the field. Tokens, not sentences. */
  const ANSWERS: Readonly<Record<string, readonly string[]>> = {
    is: ['open', 'done', 'archived', 'trashed'],
    type: ['task', 'work_package', 'activity'],
    who: ['me', 'none'],
    with: ['me'],
    by: ['me'],
    due: ['overdue', 'today', 'week', 'month', 'none'],
    start: ['today', 'week', 'none'],
    created: ['today', 'week'],
    updated: ['today', 'week'],
    done: ['today', 'week', 'none'],
    sort: Object.keys(SORTS),
  };

  const KEY_NAMES = Object.keys(KEYS) as Key[];

  let focused = $state(false);
  let showRequest = $state(false);
  let field = $state<HTMLInputElement | null>(null);

  /** The chunk the caret is in, which is what the suggestions are about. */
  let caret = $state(0);

  const editing = $derived.by(() => {
    const upTo = value.slice(0, caret);
    const start = Math.max(upTo.lastIndexOf(' ') + 1, 0);
    return value.slice(start, caret);
  });

  /**
   * What to offer: the keys whose name the caret's chunk begins, or — once a colon is typed — the
   * answers of the key it names. The installation's fields decide which keys exist at all.
   */
  const offered = $derived.by(() => {
    const stem = editing.replace(/^-/, '');
    const colon = stem.indexOf(':');
    if (colon < 0) {
      const partial = stem.toLowerCase();
      return KEY_NAMES.filter(
        (key) => (KEYS[key] === '' || context.fields.has(KEYS[key])) && key.startsWith(partial),
      ).map((key) => ({ insert: `${key}:`, word: `${key}:`, note: t(`app.searchline.key.${key}`) }));
    }
    const key = stem.slice(0, colon);
    const written = stem.slice(colon + 1).toLowerCase();
    // An answer already written whole is not a suggestion: a list that stays open over a finished
    // token is a list covering the chips it just produced.
    if ((ANSWERS[key] ?? []).includes(written)) return [];
    return (ANSWERS[key] ?? [])
      .filter((answer) => answer.startsWith(written))
      .map((answer) => ({ insert: `${key}:${answer} `, word: `${key}:${answer}`, note: '' }));
  });

  /** Replaces the chunk the caret is in, which is what accepting a suggestion does. */
  function accept(insert: string) {
    const upTo = value.slice(0, caret);
    const start = Math.max(upTo.lastIndexOf(' ') + 1, 0);
    const negated = value.slice(start, caret).startsWith('-') ? '-' : '';
    const next = `${value.slice(0, start)}${negated}${insert}${value.slice(caret)}`;
    onchange(next);
    const at = start + negated.length + insert.length;
    queueMicrotask(() => {
      field?.focus();
      field?.setSelectionRange(at, at);
      caret = at;
    });
  }

  function track(event: Event) {
    const input = event.currentTarget as HTMLInputElement;
    field = input;
    caret = input.selectionStart ?? input.value.length;
  }

  /** The request, as the screen will send it. Shown because the claim is that nothing is invented. */
  const request = $derived(
    JSON.stringify(
      {
        ...(compiled.q === undefined ? {} : { q: compiled.q }),
        ...(compiled.filter === undefined ? {} : { filter: compiled.filter }),
        ...(compiled.sort === undefined ? {} : { sort: compiled.sort }),
        ...(compiled.includeArchived ? { include_archived: true } : {}),
        ...(compiled.includeTrashed ? { include_trashed: true } : {}),
      },
      null,
      2,
    ),
  );
</script>

<Stack gap="100">
  <div class="line">
    <SearchField
      label={t('app.searchline.label')}
      clearLabel={t('app.search.clear')}
      placeholder={t('app.searchline.placeholder')}
      {value}
      oninput={(event) => {
        track(event);
        onchange((event.currentTarget as HTMLInputElement).value);
      }}
      onkeyup={track}
      onclick={track}
      onfocus={(event) => {
        focused = true;
        track(event);
      }}
      onblur={() => setTimeout(() => (focused = false), 150)}
      onclear={() => onchange('')}
    />

    {#if focused && offered.length > 0}
      <!-- Under the field rather than over the page: it is a list of the language's own words, and
           a reader who is typing should still see what they have already narrowed to. -->
      <div class="offers" role="listbox" aria-label={t('app.searchline.suggestions')}>
        {#each offered as offer (offer.word)}
          <button type="button" class="offer" onmousedown={() => accept(offer.insert)}>
            <code>{offer.word}</code>
            {#if offer.note}<span class="note">{offer.note}</span>{/if}
          </button>
        {/each}
      </div>
    {/if}
  </div>

  {#if parsed.tokens.length > 0}
    <div class="drawn" role="group" aria-label={t('app.searchline.narrowing')}>
      {#each parsed.tokens as token (write(token))}
        <button
          type="button"
          class="token"
          aria-label={t('app.searchline.remove', { part: write(token) })}
          onclick={() => onchange(toggle(value, token.key as Key, token.value, token.negated))}
        >
          <code>{write(token)}</code>
          <!-- A plain mark, not a `Badge`. A badge is a label with padding of its own, so the ×
               carried a pill's worth of space on both sides inside a chip that already has
               padding - which reads as a gap after the × rather than as a control. -->
          <span class="remove" aria-hidden="true">×</span>
        </button>
      {/each}
    </div>
  {/if}

  {#each compiled.problems as problem (problem.code + JSON.stringify(problem.params ?? {}))}
    <p class="problem" role="status">{t(problem.code, problem.params)}</p>
  {/each}

  <div class="aside">
    <p class="hint">{t('app.searchline.hint')}</p>
    <Button size="sm" tone="subtle" onclick={() => (showRequest = !showRequest)}>
      {t(showRequest ? 'app.searchline.request_hide' : 'app.searchline.request_show')}
    </Button>
  </div>

  {#if showRequest}
    <pre class="request">{request}</pre>
  {/if}
</Stack>

<style>
  .line { position: relative; }

  .offers {
    position: absolute;
    inset-inline: 0;
    z-index: 1;
    display: flex;
    flex-direction: column;
    max-block-size: var(--sp-800);
    overflow-y: auto;
    margin-block-start: var(--sp-050);
    border: var(--bw-hairline) solid var(--border-default);
    border-radius: var(--r-md);
    background: var(--bg-surface);
    box-shadow: var(--shadow-overlay);
  }

  .offer {
    display: flex;
    gap: var(--sp-150);
    align-items: baseline;
    padding: var(--sp-100) var(--sp-150);
    border: 0;
    background: none;
    color: var(--text-primary);
    font: inherit;
    font-size: var(--fs-075);
    text-align: start;
    cursor: pointer;
  }

  .offer:hover { background: var(--bg-surface-sunken); }

  .note { color: var(--text-secondary); }

  .drawn { display: flex; flex-wrap: wrap; gap: var(--sp-100); }

  .token {
    display: inline-flex;
    align-items: center;
    gap: var(--sp-050);
    min-block-size: var(--density-control-sm-min);
    /* Less at the end than at the start: the × is the last thing in the chip and carries its own
       optical space, so an equal inset reads as too much. */
    padding-inline: var(--sp-150) var(--sp-100);
    border: var(--bw-hairline) solid var(--accent-primary);
    border-radius: var(--r-full);
    background: var(--bg-surface);
    color: var(--text-primary);
    font: inherit;
    font-size: var(--fs-075);
    cursor: pointer;
  }

  .token:focus-visible,
  .offer:focus-visible {
    outline: var(--bw-ring) solid var(--focus-ring);
    outline-offset: var(--sp-025);
  }

  .remove { color: var(--text-secondary); line-height: var(--lh-tight); }

  .token:hover .remove { color: var(--text-primary); }

  code { font-family: var(--font-mono); }

  .problem { margin: 0; max-width: 64ch; color: var(--text-danger); font-size: var(--fs-075); }

  .aside { display: flex; flex-wrap: wrap; gap: var(--sp-150); align-items: center; }

  .hint { margin: 0; max-width: 64ch; color: var(--text-secondary); font-size: var(--fs-075); }

  .request {
    margin: 0;
    padding: var(--sp-150);
    overflow-x: auto;
    border-radius: var(--r-md);
    background: var(--bg-surface-sunken);
    color: var(--text-secondary);
    font-family: var(--font-mono);
    font-size: var(--fs-075);
  }
</style>
