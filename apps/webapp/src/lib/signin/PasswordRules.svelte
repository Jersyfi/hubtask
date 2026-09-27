<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // What this password still has to be, as the workspace's own rules.
  //
  // **Nobody wrote these sentences.** The server answers which switches are on and with what
  // numbers; each becomes a message code with parameters, and the renderer makes the sentence in
  // the reader's language (ADR-0011). A workspace that turns a switch on gains a line without
  // anybody touching a catalogue, and a translator sees one entry per rule rather than one per
  // combination.
  //
  // **A mark, a word, and only then a colour.** Rule 3: the state is carried by the character and
  // by the name a screen reader is given; the colour is the echo. In greyscale the list still
  // reads.
  //
  // **It is a description, not an announcement.** The list is pointed at by the field's
  // `aria-describedby`, so it is read when the field is reached rather than shouted at every
  // keystroke. The one thing that *is* announced is the count, once, when a send is refused -
  // `SignInCard` owns that, because the frame owns the live region.

  import { Icon } from '@hubtask/design-system/components';
  import type { IconName } from '@hubtask/design-system/icons';

  import { t } from '../i18n/i18n.svelte.ts';
  import type { RuleLine } from '../data/signinrules.ts';

  interface Props {
    /** The lines, in the order `evaluate` produced them. */
    lines: readonly RuleLine[];
    /** The id the field points at. */
    id: string;
  }

  const { lines, id }: Props = $props();

  /** The mark each state carries. Two of the set's; a line still waiting carries none. */
  const MARKS: Partial<Record<RuleLine['state'], IconName>> = { met: 'check', failed: 'circle-x' };
</script>

<ul class="rules" {id}>
  {#each lines as line (line.id)}
    {@const mark = MARKS[line.state]}
    <li class="rule" data-state={line.state}>
      <span class="mark" aria-hidden="true">
        {#if mark}
          <Icon name={mark} size="sm" />
        {:else}
          <span class="dot"></span>
        {/if}
      </span>
      <span class="what">
        {t(line.code, line.params)}
        <!-- The state in words, for a reader who hears the list rather than sees it. Visible text
             carries it too, for the two states where a person needs to know that something is
             still happening rather than already decided. -->
        {#if line.state === 'checking'}
          <span class="aside">{t('app.password.checking')}</span>
        {:else if line.state === 'unchecked'}
          <span class="aside">{t('app.password.unchecked')}</span>
        {:else}
          <span class="visually-hidden">
            {line.state === 'met' ? t('app.password.met') : t('app.password.not_met')}
          </span>
        {/if}
      </span>
    </li>
  {/each}
</ul>

<style>
  .rules {
    margin: 0;
    padding: 0;
    list-style: none;
    display: grid;
    gap: var(--sp-050);
    max-width: 60ch;
  }

  .rule {
    display: flex;
    align-items: baseline;
    gap: var(--sp-100);
    font-size: var(--fs-075);
    line-height: var(--lh-snug);
  }

  /* A line already met steps back; one still open keeps the reader's colour. At twelve lines this
     is what makes the list readable - only what is left stands out, and nothing disappears. */
  .rule[data-state='met'] { color: var(--text-subtle); }
  .rule[data-state='unmet'],
  .rule[data-state='server'],
  .rule[data-state='checking'] { color: var(--text-secondary); }
  .rule[data-state='failed'] { color: var(--text-danger); }
  .rule[data-state='unchecked'] { color: var(--text-secondary); }

  .mark {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: var(--sp-200);
    flex: none;
  }

  .rule[data-state='met'] .mark { color: var(--text-success); }
  .rule[data-state='failed'] .mark { color: var(--text-danger); }

  /* The waiting mark: a ring rather than a character, so a line nobody has answered yet does not
     look like a line that failed. */
  .dot {
    width: var(--sp-100);
    height: var(--sp-100);
    border: var(--bw-hairline) solid currentColor;
    border-radius: var(--r-full);
    opacity: 0.6;
  }

  .rule[data-state='checking'] .dot { border-color: var(--accent-primary); opacity: 1; }

  .aside {
    margin-inline-start: var(--sp-050);
    color: var(--text-subtle);
    font-family: var(--font-mono);
    font-size: var(--fs-050);
  }

  .visually-hidden {
    position: absolute;
    width: var(--bw-hairline);
    height: var(--bw-hairline);
    padding: 0;
    margin: calc(-1 * var(--bw-hairline));
    overflow: hidden;
    clip-path: inset(50%);
    white-space: nowrap;
  }
</style>
