<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The device's list of workspaces as rows (UC-ID-25, the approved CD-09 preview): a mark, the
  // name, the host under it. Drawn two ways from one list. In *Switch workspace* a row opens with
  // *Open* - the workspace a person is in says so instead - and keeps forgetting in its menu; at the
  // installation's own address a row is an arrow and a cross. An entry whose host no longer answers
  // says so and offers *Forget* alone, wherever it is drawn (check 6). An installed app's list draws
  // the same rows (check 7): one it is signed out of says so and offers *Sign in* (the approved
  // preview), and its menu forgets the workspace in the app's words.

  import { Badge, Button, IconButton, Menu } from '@hubtask/design-system/components';

  import { t } from '../i18n/i18n.svelte.ts';

  /** A row: the browser's entry and whether it answers, or an installed app's workspace. */
  interface Row {
    /** What the row is keyed by: the signed entry, or the app's server. */
    readonly entry: string;
    readonly host: string;
    readonly name: string;
    readonly answers?: boolean | undefined;
    /** An installed app holds no session for it: *Sign in* in place of *Open*. */
    readonly isSignedOut?: boolean;
  }

  interface Props {
    rows: readonly Row[];
    /** The host this page is at, whose row is *This workspace*; nothing at the installation's address. */
    here?: string;
    /** The installation's own address: the arrow and the cross, no menu. */
    isHome?: boolean;
    onopen: (host: string) => void;
    onforget: (host: string) => void;
    /** An installed app's *Sign in*, for a row it holds no session for. */
    onsignin?: (host: string) => void;
    /** The menu's row, where it is not the browser's *Forget this workspace on this device*. */
    forgetLabel?: string;
  }

  const { rows, here, isHome = false, onopen, onforget, onsignin, forgetLabel }: Props = $props();

  /** First and last word's first letter: a mark, not a picture, so no two workspaces look alike by colour. */
  function initials(name: string): string {
    const words = name.trim().split(/\s+/).filter(Boolean);
    const first = [...(words[0] ?? '')][0] ?? '';
    const last = words.length > 1 ? ([...(words[words.length - 1] ?? '')][0] ?? '') : '';
    return `${first}${last}`.toUpperCase();
  }
</script>

<ul class="list">
  {#each rows as row (row.entry)}
    {@const isGone = row.answers === false}
    <li class:gone={isGone}>
      <span class="mark" aria-hidden="true">{initials(row.name)}</span>
      <div class="grow">
        <span class="name">{row.name}</span>
        <span class="address">{row.host}</span>
        {#if isGone}<span class="note">{t('app.workspaces.gone')}</span>
        {:else if row.isSignedOut}<span class="note">{t('app.app_workspaces.signed_out')}</span>{/if}
      </div>
      {#if isGone}
        <Button tone="subtle" size="sm" onclick={() => onforget(row.host)} aria-label={t('app.workspaces.forget_named', { name: row.name })}>
          {t('app.workspaces.forget')}
        </Button>
      {:else if isHome}
        <IconButton icon="arrow-right" label={t('app.workspaces.open_named', { name: row.name })} size="sm" onclick={() => onopen(row.host)} />
        <IconButton icon="x" label={t('app.workspaces.forget_named', { name: row.name })} size="sm" onclick={() => onforget(row.host)} />
      {:else}
        {#if row.isSignedOut && onsignin}
          <Button tone="secondary" size="sm" onclick={() => onsignin(row.host)} aria-label={t('app.app_workspaces.sign_in_named', { name: row.name })}>
            {t('app.app_workspaces.sign_in')}
          </Button>
        {:else if row.host === here}
          <Badge>{t('app.workspaces.this_one')}</Badge>
        {:else}
          <Button tone="secondary" size="sm" onclick={() => onopen(row.host)} aria-label={t('app.workspaces.open_named', { name: row.name })}>
            {t('app.workspaces.open')}
          </Button>
        {/if}
        <Menu
          label={t('app.workspaces.actions', { name: row.name })}
          items={[{ id: 'forget', label: forgetLabel ?? t('app.workspaces.forget_here'), isDestructive: forgetLabel !== undefined }]}
          placement={{ side: 'block-end', align: 'end' }}
          onselect={() => onforget(row.host)}
        >
          {#snippet trigger(props)}
            <IconButton icon="ellipsis" label={t('app.workspaces.actions', { name: row.name })} size="sm" {...props} />
          {/snippet}
        </Menu>
      {/if}
    </li>
  {/each}
</ul>

<style>
  .list {
    display: flex;
    flex-direction: column;
    margin: 0;
    padding: 0;
    list-style: none;
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-md);
    overflow: hidden;
  }

  li {
    display: flex;
    align-items: center;
    gap: var(--sp-150);
    padding: var(--sp-150);
    background: var(--bg-surface);
    border-block-end: var(--bw-hairline) solid var(--border-subtle);
  }

  li:last-child { border-block-end: 0; }

  .grow { flex: 1; min-width: 0; }

  .mark {
    display: inline-grid;
    place-items: center;
    flex: none;
    inline-size: var(--sp-400);
    block-size: var(--sp-400);
    border-radius: var(--r-md);
    background: var(--bg-surface-sunken);
    color: var(--text-secondary);
    font-size: var(--fs-075);
    font-weight: var(--fw-semibold);
  }

  .name { display: block; font-weight: var(--fw-medium); overflow-wrap: anywhere; }

  .address {
    display: block;
    font-family: var(--font-mono);
    font-size: var(--fs-075);
    color: var(--text-subtle);
    overflow-wrap: anywhere;
  }

  .note {
    display: block;
    margin-block-start: var(--sp-050);
    font-size: var(--fs-075);
    color: var(--text-subtle);
  }

  .gone .name { color: var(--text-subtle); }
</style>
