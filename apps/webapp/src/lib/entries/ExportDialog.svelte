<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A view's rows, as a file.
  //
  // **Nothing of the query reaches a URL.** The export is a `POST` through the seam's `document`,
  // for the reason `/search` is one: what a view selects is the caller's content, and a query
  // string travels through access logs, proxies and browser history. The bytes come back and
  // `platform.saveFile` hands them over.
  //
  // **A truncated file says so.** A result that reached `max_export_rows` is answered whole *up to
  // the cap*, and handing it over quietly would hand somebody a file that looks complete.
  //
  // **The zone is the reader's**, which is what the contract defaults to and what makes a date in
  // the file read the way it reads on screen.

  import { Button, Dialog, Inline, Select, Stack } from '@hubtask/design-system/components';
  import type { SavedView } from '@hubtask/sync-engine';

  import { actor } from '../data/account.svelte.ts';
  import { manifest } from '../data/capabilities.svelte.ts';
  import { views } from '../data/views.svelte.ts';
  import { EXPORT_FORMATS, exportFileName, isTruncated, type ExportFormat } from '../data/views.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { platform } from '../platform/index.ts';
  import { renderProblem } from '../problem.ts';

  interface Props {
    /** The view being exported. Present opens the dialog. */
    view?: SavedView;
    onclose: () => void;
  }

  const { view, onclose }: Props = $props();

  let format = $state<ExportFormat>('CSV');
  let isExporting = $state(false);
  let failure = $state<string | undefined>(undefined);
  let notice = $state<string | undefined>(undefined);
  let truncated = $state(false);

  const cap = $derived((manifest.value?.limits as Record<string, unknown> | undefined)?.['max_export_rows']);

  $effect(() => {
    if (!view) return;
    format = 'CSV';
    failure = undefined;
    notice = undefined;
    truncated = false;
  });

  async function run() {
    if (!view) return;
    isExporting = true;
    failure = undefined;
    notice = undefined;
    truncated = false;
    try {
      const document = await views.export(view.id, { format, time_zone: actor.zone });
      truncated = isTruncated(document.headers);
      // The server's own name where it gave one, and the view's name where it did not: a download
      // called `export` says nothing about which of somebody's six views it is.
      const fileName = document.fileName ?? exportFileName(view.name, format);
      platform.saveFile(document.body, fileName);
      notice = t('app.export.done', { name: fileName });
    } catch (error) {
      failure = renderProblem(error as never, messages).message;
    } finally {
      isExporting = false;
    }
  }
</script>

<Dialog
  isOpen={view !== undefined}
  title={view ? t('app.export.title') : t('app.export.title')}
  dismissLabel={t('app.workspace.cancel')}
  onClose={onclose}
>
  <Stack gap="150">
    <Select
      label={t('app.export.format')}
      bind:value={format}
      options={EXPORT_FORMATS.map((each) => ({ value: each, label: t(`app.export.format_${each}`) }))}
    />
    {#if format === 'ICS'}
      <!-- The contract's own rule, said before the file arrives rather than discovered from a
           short one: an entry with no due date is not a calendar entry. -->
      <p class="quiet">{t('app.export.ics_note')}</p>
    {/if}

    {#if notice}<p class="quiet">{notice}</p>{/if}
    {#if truncated}
      <p class="warning">{t('app.export.truncated', { maximum: String(cap ?? '') })}</p>
    {/if}
    {#if failure}<p class="failure">{failure}</p>{/if}

    <Inline gap="100">
      <Button isBusy={isExporting} busyLabel={t('app.workspace.saving')} onclick={() => void run()}>
        {t('app.export.go')}
      </Button>
      <Button tone="secondary" onclick={onclose}>{t('app.workspace.cancel')}</Button>
    </Inline>
  </Stack>
</Dialog>

<style>
  .quiet { margin: 0; color: var(--text-secondary); font-size: var(--fs-075); max-width: 64ch; }

  .warning { margin: 0; color: var(--text-warning); max-width: 64ch; }

  .failure { margin: 0; color: var(--text-danger); font-size: var(--fs-075); }
</style>
