<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A plan brought into a collection from a project file (UC-PLN-09), in the import's four steps
  // (user-experience.md §7.11): where from, the file, the check - a dry run said in words through
  // `PlanSummary` -, and the import with its report. All or nothing: every entry and dependency is
  // written through its own use case on the server.
  //
  // **The file decides its format.** A project tool's XML chosen as CSV would be refused or, worse,
  // read as one odd row: what the file is (`projectFormatOf`) wins over the card, and the check says
  // which format it was read as.

  import { Button, Inline, PlanSummary, Stack, TemplateCard, TemplateGallery, UploadField, VisuallyHidden } from '@hubtask/design-system/components';
  import type { PlanImport } from '@hubtask/sync-engine';

  import { announcer } from '../announce.svelte.ts';
  import { engine } from '../data/engine.ts';
  import { namesOf } from '../data/kudos.ts';
  import { projectFileOf, projectPlanOf } from '../data/planning.ts';
  import { schedulePath } from '../data/planning.svelte.ts';
  import { formatBytes } from '../i18n/bytes.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';

  /** What the contract takes (`content`'s maxLength); a larger file is refused before it is sent. */
  const MAX_BYTES = 512 * 1024;

  type Format = 'MSPROJECT_XML' | 'CSV';
  const FORMATS: readonly Format[] = ['MSPROJECT_XML', 'CSV'];
  const ACCEPT: Record<Format, string> = { MSPROJECT_XML: '.xml,application/xml,text/xml', CSV: '.csv,text/csv' };

  type Step = 'kind' | 'file' | 'check' | 'run';
  const STEPS: readonly Step[] = ['kind', 'file', 'check', 'run'];

  interface Props {
    collectionId: string;
    zone: string;
    ondone: () => void;
  }

  const { collectionId, zone, ondone }: Props = $props();

  let step = $state<Step>('kind');
  let format = $state<Format>('MSPROJECT_XML');
  let chosen = $state<File | null>(null);
  let content = $state('');
  let preview = $state<PlanImport | undefined>(undefined);
  let report = $state<PlanImport | undefined>(undefined);
  let failure = $state<string | undefined>(undefined);
  let missing = $state(false);
  let isBusy = $state(false);

  const words = $derived({
    t: (code: string, params?: Record<string, string>) => t(code, params),
    list: (parts: readonly string[]) => namesOf(parts, messages.locale),
  });
  const heading = $derived(
    t('app.import.step', { number: String(STEPS.indexOf(step) + 1), of: String(STEPS.length), name: t(`app.import.step_${step}`) }),
  );
  const limit = formatBytes(MAX_BYTES, messages.locale);

  function go(next: Step) {
    step = next;
    failure = undefined;
    announcer.say(t('app.import.step', { number: String(STEPS.indexOf(next) + 1), of: String(STEPS.length), name: t(`app.import.step_${next}`) }));
  }

  function pick(next: Format) {
    if (next !== format) {
      chosen = null;
      content = '';
      preview = undefined;
    }
    format = next;
    go('file');
  }

  async function readFile(file: File) {
    preview = undefined;
    failure = undefined;
    missing = false;
    chosen = file;
    content = '';
    if (file.size > MAX_BYTES) return;
    ({ content, format } = projectFileOf(file.name, new Uint8Array(await file.arrayBuffer())));
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
      if (dryRun) {
        preview = answer;
        announcer.say(t('app.import.checked_announced'));
      } else {
        report = answer;
        go('run');
      }
    } catch (error) {
      failure = renderProblem(error as never, messages).message;
    } finally {
      isBusy = false;
    }
  }

  /** The file being read: *Next* pressed meanwhile waits for it rather than calling it missing. */
  let reading: Promise<void> | undefined;

  async function toCheck() {
    await reading;
    if (!content) {
      missing = true;
      return;
    }
    go('check');
    void send(true);
  }
</script>

<Stack gap="150">
  <h3 class="step" aria-live="polite">{heading}</h3>

  {#if step === 'kind'}
    <TemplateGallery label={t('app.planning.import_sources')} isLabelHidden>
      {#each FORMATS as each (each)}
        <TemplateCard name={t(`app.planning.import_format_${each}`)} summary={t(`app.planning.import_format_${each}_card`)} mark="file-text" isCompact data-plan-format={each}>
          {#snippet action()}
            <Button size="sm" tone="secondary" onclick={() => pick(each)}>
              {t('app.import.choose_source')}<VisuallyHidden>{` ${t(`app.planning.import_format_${each}`)}`}</VisuallyHidden>
            </Button>
          {/snippet}
        </TemplateCard>
      {/each}
    </TemplateGallery>
  {:else if step === 'file'}
    <UploadField
      label={t('app.planning.import_file_of', { format: t(`app.planning.import_format_${format}`) })}
      hint={t('app.planning.import_limit', { limit })}
      chooseLabel={t('app.import.choose')}
      dropLabel={t('app.media.drop')}
      accept={ACCEPT[format]}
      file={chosen}
      sizeLabel={chosen ? t('app.media.size_of_limit', { size: formatBytes(chosen.size, messages.locale), limit }) : undefined}
      overLimitLabel={chosen && chosen.size > MAX_BYTES ? t('app.media.over_limit') : undefined}
      error={missing && !content ? t('app.import.file_first') : undefined}
      onChoose={(file) => (reading = readFile(file))}
    />
    <Inline gap="100">
      <Button tone="secondary" onclick={() => go('kind')}>{t('app.import.back')}</Button>
      <Button tone="primary" onclick={() => void toCheck()}>{t('app.import.next')}</Button>
    </Inline>
  {:else if step === 'check'}
    <p class="quiet">{t('app.planning.import_read_as', { format: t(`app.planning.import_format_${format}`) })}</p>
    {#if isBusy && !preview}
      <p class="quiet" role="status">{t('app.import.checking')}</p>
    {:else if preview}
      <PlanSummary label={t('app.import.plan_label_dry')} lines={projectPlanOf(preview, true, words)} emptyLabel={t('app.import.plan_empty')} reach={t('app.planning.import_nothing_yet')} />
    {/if}
    {#if failure}<p class="failure" role="alert">{failure}</p>{/if}
    <Inline gap="100">
      <Button tone="secondary" onclick={() => go('file')}>{t('app.import.back')}</Button>
      {#if preview}
        <Button tone="primary" {isBusy} busyLabel={t('app.import.starting')} onclick={() => void send(false)}>{t('app.planning.import_run')}</Button>
      {/if}
    </Inline>
  {:else}
    {#if report}
      <PlanSummary label={t('app.import.plan_label_done')} lines={projectPlanOf(report, false, words)} emptyLabel={t('app.import.plan_empty_done')} />
    {/if}
    <Inline gap="100">
      <Button tone="primary" onclick={ondone}>{t('app.planning.import_see_plan')}</Button>
    </Inline>
  {/if}
</Stack>

<style>
  .step { margin: 0; font-family: var(--font-display); font-size: var(--fs-100); font-weight: var(--fw-semibold); color: var(--text-secondary); }
  .quiet { margin: 0; color: var(--text-secondary); font-size: var(--fs-075); max-width: 64ch; }
  .failure { margin: 0; color: var(--text-danger); font-size: var(--fs-075); }
</style>
