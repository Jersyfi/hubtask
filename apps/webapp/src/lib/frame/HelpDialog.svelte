<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // *Help and shortcuts* (UC-WRK-27 check 3, ux-inventory.md §E1): every keyboard shortcut, read
  // from the one list (`lib/shortcuts.ts`), and the tour, which was a row of the account group of
  // its own. `?` opens it, and so do the account group and the palette. *Getting help* comes first:
  // the details a support request needs, copied in one press (user-experience.md §5.8).

  import { untrack } from 'svelte';
  import { Button, Dialog, Kbd, Stack } from '@hubtask/design-system/components';

  import { accounts } from '../data/accounts.svelte.ts';
  import { manifest } from '../data/capabilities.svelte.ts';
  import { people } from '../data/people.svelte.ts';
  import { workspace } from '../data/workspace.svelte.ts';
  import { SHORTCUTS, type Scope } from '../shortcuts.ts';
  import { recentReferences, supportDetails } from '../support.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { toast } from './toasts.svelte.ts';
  import { productOf } from '../navigation.ts';

  interface Props {
    isOpen: boolean;
    /** Starts the tour again, once this has closed. */
    ontour: () => void;
    /** The product whose home is open, whose own tour is offered beside (user-experience.md §3.4). */
    product?: string;
    onproducttour?: (module: string) => void;
  }

  let { isOpen = $bindable(false), ontour, product, onproducttour }: Props = $props();
  const productRow = $derived(product ? productOf(product) : undefined);

  const SCOPES: readonly Scope[] = ['everywhere', 'palette', 'list', 'selection'];

  // Where this workspace's people ask and read, set by the workspace or the installation above it;
  // a contact is a page or an address to write to.
  const support = $derived(manifest.value?.legal?.support_url);
  const docs = $derived(manifest.value?.legal?.docs_url);
  const supportWord = $derived(support?.startsWith('mailto:') ? support.slice('mailto:'.length) : support);

  // The workspace's short name, read while the dialog is open; a reader the read refuses copies a dash.
  $effect(() => {
    if (!isOpen) return;
    return untrack(() => workspace.open());
  });

  // Who administers this workspace: its owners and administrators, by name, a few at most - a
  // person to ask. A group holding the role is not a person, so it is left out.
  const ADMINISTRATORS_SHOWN = 3;
  $effect(() => {
    if (!isOpen) return;
    return untrack(() => people.openScope({ scopeType: 'TENANT' }));
  });
  const administrators = $derived(isOpen
    ? [...new Set(people.along({})
      .filter((each) => each.scope_type === 'TENANT' && (each.role === 'OWNER' || each.role === 'ADMIN') && each.account_id)
      .map((each) => each.account_id as string))]
    : []);
  $effect(() => accounts.resolve(administrators));
  const administratorNames = $derived(administrators
    .map((id) => accounts.nameOf(id))
    .filter((name): name is string => !!name)
    .slice(0, ADMINISTRATORS_SHOWN));

  function copyDetails() {
    const text = supportDetails({
      version: manifest.value?.product_version,
      workspace: workspace.workspace?.slug,
      route: `${location.pathname}${location.search}`,
      references: recentReferences(),
      labels: {
        version: t('app.help.detail_version'),
        workspace: t('app.help.detail_workspace'),
        route: t('app.help.detail_route'),
        references: t('app.help.detail_references'),
      },
    });
    void navigator.clipboard
      .writeText(text)
      .then(() => toast.show({ text: t('app.help.copied'), tone: 'success' }))
      .catch(() => {
        // A clipboard the browser refuses is said nowhere: the dialog stays, and so does the button.
      });
  }

  function takeTour() {
    isOpen = false;
    // After the dialog has returned focus: the tour's first step takes it from there.
    queueMicrotask(ontour);
  }

  function takeProductTour(module: string) {
    isOpen = false;
    queueMicrotask(() => onproducttour?.(module));
  }
</script>

<Dialog size="l" title={t('app.help.label')} dismissLabel={t('app.palette.close')} bind:isOpen>
  <Stack gap="300">
    <section aria-labelledby="help-support">
      <h3 class="group" id="help-support">{t('app.help.support_title')}</h3>
      <Stack gap="100">
        <p class="body">{t(support ? 'app.help.support_body_contact' : 'app.help.support_body')}</p>
        {#if support || docs || administratorNames.length > 0}
          <dl class="contacts" data-help-contacts>
            {#if administratorNames.length > 0}
              <div data-help-administrators>
                <dt>{t('app.help.administered_by')}</dt>
                <dd>{new Intl.ListFormat(messages.locale, { type: 'conjunction' }).format(administratorNames)}{administrators.length > ADMINISTRATORS_SHOWN ? ` ${t('app.help.and_more', { count: String(administrators.length - ADMINISTRATORS_SHOWN) })}` : ''}</dd>
              </div>
            {/if}
            {#if support}
              <div><dt>{t('app.help.support_contact')}</dt><dd><a href={support} target="_blank" rel="noopener">{supportWord}</a></dd></div>
            {/if}
            {#if docs}
              <div><dt>{t('app.help.documentation')}</dt><dd><a href={docs} target="_blank" rel="noopener">{docs}</a></dd></div>
            {/if}
          </dl>
        {/if}
        <div><Button icon="copy" onclick={copyDetails}>{t('app.help.copy_details')}</Button></div>
      </Stack>
    </section>
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
                  <Kbd>{keys}</Kbd>
                {/each}
              </dd>
            </div>
          {/each}
        </dl>
      </section>
    {/each}
    <div class="tours">
      <Button icon="compass" onclick={takeTour}>{t('app.help.tour_again')}</Button>
      {#if productRow && onproducttour}
        <Button icon="compass" data-product-tour={productRow.module} onclick={() => takeProductTour(productRow.module)}>
          {t('app.product_tour.start', { product: t(productRow.code) })}
        </Button>
      {/if}
    </div>
  </Stack>
</Dialog>

<style>
  .tours { display: flex; flex-wrap: wrap; gap: var(--sp-100); }
  .group {
    margin: 0 0 var(--sp-050);
    font-size: var(--fs-075);
    font-weight: var(--fw-semibold);
    color: var(--text-secondary);
  }

  /* Two columns that hold: what it does at the start, wrapping where it must; the keys at the end,
     never broken across lines, so every key of the dialog stands on one edge. */
  .list { margin: 0; display: grid; grid-template-columns: minmax(0, 1fr) auto; }

  .row { display: contents; }

  dt,
  dd {
    padding-block: var(--sp-100);
    border-block-start: var(--bw-hairline) solid var(--border-subtle);
  }

  .row:first-child > dt,
  .row:first-child > dd { border-block-start: 0; }

  dt { margin: 0; padding-inline-end: var(--sp-300); color: var(--text-primary); font-size: var(--fs-100); }

  dd {
    margin: 0;
    display: inline-flex;
    align-items: center;
    justify-content: end;
    gap: var(--sp-050);
    white-space: nowrap;
  }

  .or { color: var(--text-subtle); font-size: var(--fs-075); }

  .body { margin: 0; color: var(--text-secondary); font-size: var(--fs-100); }

  .contacts { margin: 0; display: grid; gap: var(--sp-050); font-size: var(--fs-100); }
  .contacts > div { display: flex; flex-wrap: wrap; gap: var(--sp-100); }
  /* Not the shortcut rows' cells above: no rule between them, and an address may wrap. */
  .contacts dt,
  .contacts dd { display: block; padding: 0; border: 0; white-space: normal; }
  .contacts dt { color: var(--text-secondary); }
  .contacts dd { margin: 0; overflow-wrap: anywhere; }

</style>
