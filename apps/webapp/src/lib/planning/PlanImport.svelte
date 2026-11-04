<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A plan brought into a collection from a project file (UC-PLN-09): the file read in the browser,
  // its format found from its name, a dry run said in words (`PlanSummary`, user-experience.md
  // §7.11), and then the import itself - all or
  // nothing, every entry and dependency written through its own use case on the server.

  import { Banner, Button, PlanSummary, Select, Stack, UploadField, type PlanLine } from '@hubtask/design-system/components';
  import type { PlanImport } from '@hubtask/sync-engine';

  import { engine } from '../data/engine.ts';
  import { namesOf } from '../data/kudos.ts';
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

  let chosen = $state<File | null>(null);
  let content = $state('');
  let format = $state<'MSPROJECT_XML' | 'CSV'>('CSV');
  let preview = $state<PlanImport | undefined>(undefined);
  let failure = $state<string | undefined>(undefined);
  let isBusy = $state(false);

  /** The dry run in words: what the file would create, by the things a planner counts. */
  const plan = $derived.by((): PlanLine[] => {
    if (!preview) return [];
    const counted = (
      [
        ['tasks', preview.tasks], ['packages', preview.work_packages], ['activities', preview.activities],
        ['milestones', preview.milestones], ['dependencies', preview.dependencies],
      ] as const
    ).filter(([, count]) => count > 0);
    const lines: PlanLine[] = [];
    if (counted.length > 0) {
      const things = namesOf(counted.map(([word, count]) => t(`app.import.count_${word}`, { count: String(count) })), messages.locale);
      lines.push({ tone: 'add', text: t('app.import.plan_creates_dry', { things }) });
    }
    if (preview.flattened > 0) lines.push({ tone: 'change', text: t('app.planning.import_flattened', { count: String(preview.flattened) }) });
    return lines;
  });

  async function readFile(file: File) {
    preview = undefined;
    failure = undefined;
    if (file.size > MAX_BYTES) {
      failure = t('app.planning.import_too_large');
      return;
    }
    chosen = file;
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
  <UploadField
    label={t('app.planning.import_file')}
    hint={t('app.planning.import_formats')}
    chooseLabel={t('app.import.choose')}
    dropLabel={t('app.media.drop')}
    accept=".xml,.csv,text/csv,application/xml,text/xml"
    file={chosen}
    onChoose={(file) => void readFile(file)}
  />
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
    <PlanSummary label={t('app.planning.import_preview')} lines={plan} emptyLabel={t('app.import.plan_empty')} reach={t('app.planning.import_nothing_yet')} />
  {/if}
  <div class="buttons">
    <Button tone="primary" disabledReason={preview ? undefined : t('app.planning.import_choose_first')} {isBusy} onclick={() => void send(false)}>{t('app.planning.import_run')}</Button>
  </div>
</Stack>

<style>
  .buttons { display: flex; flex-wrap: wrap; gap: var(--sp-100); justify-content: flex-end; }
  .meta { color: var(--text-subtle); font-size: var(--fs-075); margin: 0; }
</style>
