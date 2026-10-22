<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // *Help and shortcuts* (UC-WRK-27 check 3, ux-inventory.md §E1): every keyboard shortcut, read
  // from the one list (`lib/shortcuts.ts`), and the tour, which was a row of the account group of
  // its own. `?` opens it, and so do the account group and the palette.

  import { Button, Dialog, Stack } from '@hubtask/design-system/components';

  import { SHORTCUTS, type Scope } from '../shortcuts.ts';
  import { t } from '../i18n/i18n.svelte.ts';

  interface Props {
    isOpen: boolean;
    /** Starts the tour again, once this has closed. */
    ontour: () => void;
  }

  let { isOpen = $bindable(false), ontour }: Props = $props();

  const SCOPES: readonly Scope[] = ['everywhere', 'palette', 'list', 'selection'];

  function takeTour() {
    isOpen = false;
    // After the dialog has returned focus: the tour's first step takes it from there.
    queueMicrotask(ontour);
  }
</script>

<Dialog title={t('app.help.label')} dismissLabel={t('app.palette.close')} bind:isOpen>
  <Stack gap="300">
    {#each SCOPES as scope (scope)}
      <section aria-labelledby={`shortcuts-${scope}`}>
        <h3 class="group" id={`shortcuts-${scope}`}>{t(`app.shortcuts.group.${scope}`)}</h3>
        <dl class="list">
          {#each SHORTCUTS.filter((each) => each.scope === scope) as shortcut (shortcut.id)}
            <div class="row">
              <dt>{t(shortcut.code)}</dt>
              <dd>
                {#each shortcut.drawn as keys, index (keys)}
                  {#if index > 0}<span class="or">{t('app.shortcuts.or')}</span>{/if}
                  <kbd>{keys}</kbd>
                {/each}
              </dd>
            </div>
          {/each}
        </dl>
      </section>
    {/each}
    <div>
      <Button icon="compass" onclick={takeTour}>{t('app.help.tour_again')}</Button>
    </div>
  </Stack>
</Dialog>

<style>
  .group { margin: 0 0 var(--sp-100); font-size: var(--fs-075); font-weight: var(--fw-semibold); color: var(--text-secondary); }
  .list { margin: 0; display: grid; gap: var(--sp-100); }
  .row { display: flex; flex-wrap: wrap; align-items: baseline; justify-content: space-between; gap: var(--sp-100); }
  dt { margin: 0; }
  dd { margin: 0; display: inline-flex; flex-wrap: wrap; align-items: baseline; gap: var(--sp-050); }
  .or { color: var(--text-secondary); font-size: var(--fs-075); }
  kbd {
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-sm);
    padding-inline: var(--sp-050);
    color: var(--text-secondary);
    font-size: var(--fs-075);
  }
</style>
