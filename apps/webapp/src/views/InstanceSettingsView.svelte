<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The installation's own values, and where each lock came from (SI-17, ADR-0070 §2).
  //
  // **Read here, written through `hubctl` or the file.** ADR-0070 §5 has three doors onto one API,
  // and this screen is the one that shows what is in force — the switches by name, their values,
  // and whether a workspace may tighten them. Writing eighteen typed switches from a form is a
  // screen of its own and is not what SI-17 asks for; what an operator needs first is to *see*
  // which of them are locked, because a locked switch is the one a workspace will ask about.
  //
  // `source` says which door is in force: a file in `enforce` mode makes this whole level
  // read-only, and a screen that offered controls against it would be offering a refusal.

  import { untrack } from 'svelte';

  import { Badge, Banner, PageHeader, Spinner, Stack, Table } from '@hubtask/design-system/components';

  import InstanceGate from '../lib/instance/InstanceGate.svelte';
  import { instance } from '../lib/data/instance.svelte.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { renderProblem } from '../lib/problem.ts';
  import { page } from '../lib/frame/page.svelte.ts';
  import { viewport } from '../lib/frame/viewport.svelte.ts';

  interface Props {
    onnavigate: (path: string) => void;
  }

  const { onnavigate }: Props = $props();

  $effect(() => {
    const stop = untrack(() => instance.openSettings());
    return stop;
  });

  const reading = $derived(instance.settings);
  const settings = $derived(reading.status === 'ready' ? reading.data : undefined);
  const unreadable = $derived(
    reading.status === 'failed' ? renderProblem(reading.error, messages) : undefined,
  );

  const columns = $derived([
    { id: 'switch', label: t('app.instance.column_switch') },
    { id: 'value', label: t('app.instance.column_value') },
    { id: 'lock', label: t('app.instance.column_lock') },
  ]);

  /** The two areas as one list of rows, each carrying which area it came from. */
  const rows = $derived([
    ...Object.entries(settings?.sign_in ?? {}).map(([key, setting]) => ({ key: `sign_in.${key}`, setting })),
    ...Object.entries(settings?.legal ?? {}).map(([key, setting]) => ({ key: `legal.${key}`, setting })),
  ]);

  /** A value as one line. The switches are numbers, flags, strings and lists of strings. */
  function shown(value: unknown): string {
    if (Array.isArray(value)) return value.join(', ');
    if (value === null || value === undefined) return '';
    return String(value);
  }

  $effect(() => page.entitle(t('app.instance.settings')));
</script>

<Stack gap="300">
  <PageHeader title={t('app.instance.settings')} isTitleInBar={viewport.isCompact} />

  <InstanceGate onleave={() => onnavigate('/')}>
    <Stack gap="200">
      <p class="prose">{t('app.instance.settings_intro')}</p>

      {#if reading.status === 'loading' || reading.status === 'idle'}
        <p class="quiet">
          <Spinner label={t('app.instance.reading')} />
          <span>{t('app.instance.reading')}</span>
        </p>
      {:else if unreadable}
        <Banner tone="danger" title={unreadable.message}>
          {#if unreadable.reference}{t('app.error_reference', { request_id: unreadable.reference })}{/if}
        </Banner>
      {:else}
        {#if settings?.source}
          <!-- Which door is in force. A file in `enforce` mode is the answer to "why did my change
               not stick", and it belongs above the values rather than in a support conversation. -->
          <Banner tone="info">{t(`app.instance.source_${settings.source}`)}</Banner>
        {/if}
        {#if settings?.blocklist_file}
          <p class="quiet-line">{t('app.instance.blocklist_file', { path: settings.blocklist_file })}</p>
        {/if}

        {#if rows.length === 0}
          <Banner tone="info">{t('app.instance.settings_none')}</Banner>
        {:else}
          <Table label={t('app.instance.settings')} isLabelHidden columns={columns}>
            {#each rows as row (row.key)}
              <tr>
                <td class="mono">{row.key}</td>
                <td>{shown(row.setting.value)}</td>
                <td>
                  <Badge tone={row.setting.locked ? 'warning' : 'neutral'}>
                    {row.setting.locked ? t('app.instance.lock_locked') : t('app.instance.lock_open')}
                  </Badge>
                </td>
              </tr>
            {/each}
          </Table>
        {/if}
      {/if}
    </Stack>
  </InstanceGate>
</Stack>

<style>
  .prose { margin: 0; max-inline-size: 60ch; }
  .quiet { margin: 0; display: flex; align-items: center; gap: var(--sp-100); color: var(--text-secondary); }
  .quiet-line { margin: 0; color: var(--text-secondary); font-size: var(--fs-100); }
  .mono { font-family: var(--font-mono); font-size: var(--fs-100); overflow-wrap: anywhere; }
</style>
