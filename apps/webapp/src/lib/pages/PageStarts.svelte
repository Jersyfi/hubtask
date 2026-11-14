<!-- SPDX-License-Identifier: Apache-2.0 -->
<!-- Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A new page's starts (S-6, scene pnew 2): the templates as chips under the empty title, the
  // space's other templates under *All templates*, a whole space's pages on the line below. A chip
  // chooses what is previewed beside the page; the preview's button uses it.
  import { Menu } from '@hubtask/design-system/components';
  import { SHIPPED_SPACES, type ShippedSpace } from '../data/pages.svelte.ts';
  import { t } from '../i18n/i18n.svelte.ts';
  import { START_ICONS, startChips, startKey, type PageStart } from './starts.ts';

  interface Props {
    /** The space's own templates. */
    own: readonly { id: string; title: string; icon: string | null }[];
    /** The start being previewed. */
    chosen?: PageStart;
    onchoose: (start: PageStart) => void;
  }

  const { own, chosen, onchoose }: Props = $props();

  const chips = $derived(startChips(own));
  const chosenKey = $derived(chosen ? startKey(chosen) : '');
  const all = $derived([
    ...own.map((row) => ({ id: `own:${row.id}`, label: `${row.icon ? `${row.icon} ` : ''}${row.title}` })),
    ...chips.filter((chip) => chip.kind === 'shipped').map((chip) => ({ id: startKey(chip), label: labelOf(chip) })),
    ...SHIPPED_SPACES.map((key) => ({ id: `space:${key}`, label: t(`app.pages.space.${key}.title`), hasSeparatorBefore: key === SHIPPED_SPACES[0] })),
  ]);

  function labelOf(start: PageStart): string {
    if (start.kind === 'own') return `${start.icon ? `${start.icon} ` : ''}${start.title}`;
    if (start.kind === 'space') return t(`app.pages.space.${start.key}.title`);
    return `${START_ICONS[start.key]} ${t(`app.pages.template.${start.key}.title`)}`;
  }

  function chooseId(id: string) {
    const [kind, rest] = [id.slice(0, id.indexOf(':')), id.slice(id.indexOf(':') + 1)];
    if (kind === 'own') {
      const row = own.find((each) => each.id === rest);
      if (row) onchoose({ kind: 'own', id: row.id, title: row.title, icon: row.icon });
    } else if (kind === 'space') onchoose({ kind: 'space', key: rest as ShippedSpace });
    else {
      const chip = chips.find((each) => startKey(each) === id);
      if (chip) onchoose(chip);
    }
  }
</script>

<section class="starts" aria-labelledby="page-starts" data-page-starts>
  <p class="label" id="page-starts">{t('app.pages.start_with')}</p>
  <div class="chips">
    {#each chips as chip (startKey(chip))}
      <button type="button" class="chip" aria-pressed={chosenKey === startKey(chip)} data-page-start={startKey(chip)} onclick={() => onchoose(chip)}>{labelOf(chip)}</button>
    {/each}
    <Menu label={t('app.pages.all_templates')} items={all} onselect={chooseId}>
      {#snippet trigger(props)}
        <button type="button" class="chip" data-page-start-all {...props}>⋯ {t('app.pages.all_templates')}</button>
      {/snippet}
    </Menu>
  </div>
  <p class="spaces">
    {t('app.pages.whole_space')}
    {#each SHIPPED_SPACES as key, index (key)}{#if index > 0}<span aria-hidden="true">{' · '}</span>{/if}<button type="button" class="link" aria-pressed={chosenKey === `space:${key}`} data-page-start={`space:${key}`} onclick={() => onchoose({ kind: 'space', key })}>{t(`app.pages.space.${key}.title`)}</button>{/each}
  </p>
</section>

<style>
  .starts { display: grid; gap: var(--sp-100); margin-block: var(--sp-200); }
  .label { margin: 0; color: var(--text-secondary); font-size: var(--fs-075); }
  .chips { display: flex; flex-wrap: wrap; gap: var(--sp-100); }
  .chip {
    display: inline-flex; align-items: center; gap: var(--sp-050); min-block-size: var(--layout-target); padding-inline: var(--sp-150);
    border: var(--bw-hairline) solid var(--border-default); border-radius: var(--r-md); background: var(--bg-raised);
    color: var(--text-primary); font: inherit; font-size: var(--fs-090); cursor: pointer;
  }
  .chip:hover { border-color: var(--border-strong); }
  .chip[aria-pressed='true'] { border-color: var(--accent-primary); box-shadow: 0 0 0 var(--bw-hairline) var(--accent-primary); }
  .chip:focus-visible, .link:focus-visible { outline: var(--bw-ring) solid var(--focus-ring); outline-offset: var(--bw-hairline); }
  .spaces { margin: 0; color: var(--text-secondary); font-size: var(--fs-075); }
  .link { padding: 0; border: 0; background: none; color: var(--text-brand); font: inherit; cursor: pointer; }
  .link[aria-pressed='true'] { text-decoration: underline; }

  /* A thumb's target (§7): where the pointer is coarse or the window a phone's, the control's press
     reaches layout.target, by its own height or beyond its drawn box. */
  /* design-system-lint-ignore: `primitive.breakpoint.medium` (600px); a media query cannot read a custom property. */
  @media (pointer: coarse), (width < 600px) {
    button.link { position: relative; }
    button.link::after { content: ''; position: absolute; inset-block: min(0%, calc((100% - var(--layout-target)) / 2)); inset-inline: min(0%, calc((100% - var(--layout-target)) / 2)); }
    .start-all { position: relative; }
    .start-all::after { content: ''; position: absolute; inset-block: min(0%, calc((100% - var(--layout-target)) / 2)); inset-inline: min(0%, calc((100% - var(--layout-target)) / 2)); }
  }
</style>
