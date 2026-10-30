<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A plan brought into a collection from a project file (UC-PLN-09): the file read in the browser,
  // its format found from its name, a dry run's counts shown, and then the import itself - all or
  // nothing, every entry and dependency written through its own use case on the server.

  import { Banner, Button, Select, Stack } from '@hubtask/design-system/components';
  import type { PlanImport } from '@hubtask/sync-engine';

  import { engine } from '../data/engine.ts';
  import { projectFormatOf, schedulePath } from '../data/planning.svelte.ts';
  import { toast } from '../frame/toasts.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';

  /** What the contract takes (`content`'s maxLength); a larger file is refused before it is sent. */
  const MAX_BYTES = 512 * 1024;

  interface Props {
    collectionId: string;
    zone: string;
    ondone: () => void;
  }

  const { collectionId, zone, ondone }: Props = $props();

  let fileName = $state('');
  let content = $state('');
  let format = $state<'MSPROJECT_XML' | 'CSV'>('CSV');
  let preview = $state<PlanImport | undefined>(undefined);
  let failure = $state<string | undefined>(undefined);
  let isBusy = $state(false);

  async function readFile(event: Event) {
    const file = (event.currentTarget as HTMLInputElement).files?.[0];
    preview = undefined;
    failure = undefined;
    if (!file) return;
    if (file.size > MAX_BYTES) {
      failure = t('app.planning.import_too_large');
      return;
    }
    fileName = file.name;
    content = await file.text();
    format = projectFormatOf(file.name, content);
    await send(true);
  }

  async function send(dryRun: boolean) {
    if (!content) return;
    isBusy = true;
    failure = undefined;
    try {
      const answer = await engine.mutate<PlanImport>('POST', `/collections/${collectionId}/plan:import`, { format, content, zone, dry_run: dryRun }, {
        idempotencyKey: crypto.randomUUID(),
        invalidates: dryRun ? [] : [schedulePath(collectionId, zone)],
      });
      if (dryRun) preview = answer;
      else {
        toast.show({ text: t('app.planning.imported', { count: String(answer.created.length) }), tone: 'success' });
        ondone();
      }
    } catch (error) {
      failure = renderProblem(error as never, messages).message;
    } finally {
      isBusy = false;
    }
  }
</script>

<Stack gap="150">
  <p class="meta">{t('app.planning.import_hint')}</p>
  <label class="file">
    <span>{t('app.planning.import_file')}</span>
    <input type="file" accept=".xml,.csv,text/csv,application/xml,text/xml" onchange={readFile} />
    <span class="meta">{fileName || t('app.planning.import_formats')}</span>
  </label>
  {#if content}
    <Select
      label={t('app.planning.import_format')}
      options={[{ value: 'MSPROJECT_XML', label: t('app.planning.import_format_xml') }, { value: 'CSV', label: t('app.planning.import_format_csv') }]}
      bind:value={format}
      onchange={() => void send(true)}
    />
  {/if}
  {#if failure}<Banner tone="danger">{failure}</Banner>{/if}
  {#if preview}
    <section class="part" aria-labelledby="plan-import-preview">
      <h2 id="plan-import-preview" class="heading">{t('app.planning.import_preview')}</h2>
      <p class="meta">{t('app.planning.import_counts', {
        tasks: String(preview.tasks), packages: String(preview.work_packages), activities: String(preview.activities),
        dependencies: String(preview.dependencies), milestones: String(preview.milestones),
      })}</p>
      {#if preview.flattened > 0}<Banner tone="warning">{t('app.planning.import_flattened', { count: String(preview.flattened) })}</Banner>{/if}
    </section>
  {/if}
  <div class="buttons">
    <Button tone="primary" disabledReason={preview ? undefined : t('app.planning.import_choose_first')} {isBusy} onclick={() => void send(false)}>{t('app.planning.import_run')}</Button>
  </div>
</Stack>

<style>
  .file { display: grid; gap: var(--sp-050); color: var(--text-secondary); font-size: var(--fs-075); }
  .buttons { display: flex; flex-wrap: wrap; gap: var(--sp-100); justify-content: flex-end; }
  .part { display: grid; gap: var(--sp-100); }
  .heading { margin: 0; font-size: var(--fs-100); font-weight: var(--fw-medium); }
  .meta { color: var(--text-subtle); font-size: var(--fs-075); margin: 0; }
</style>
