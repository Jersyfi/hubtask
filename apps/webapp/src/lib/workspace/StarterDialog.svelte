<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The workspace's first start (UC-WRK-30 check 5, user-experience.md §3.2): asked once, of
  // whoever may shape a new workspace (`starter_offered` on `GET /tenant`), what is run here. The
  // catalogue's families come as large cards - *Simple* first, the four purposes on presets -; a
  // family opens its templates as a gallery, and a template its questions, its sample entries
  // (on by default) and its plan in words before *Set up*.
  //
  // **One command, whole or nothing.** `POST /tenant/starter:apply` makes the hub - with a template,
  // its workspace and hub parts too, as one plan and apply on the server - and closes the question
  // in one transaction: a failure leaves nothing and the question open, and a retry under the same
  // key replays rather than making a second set. A skip - or dismissing the dialog - closes it
  // with `POST /tenant/starter:close`. Nobody is asked twice.
  //
  // **Everything made is ordinary**: each hub and collection can be renamed, changed or removed,
  // and sample entries go in one press; the dialog says so.

  import { untrack } from 'svelte';

  import { Button, Dialog, Input, Spinner, Stack, TemplateCard, TemplateGallery, VisuallyHidden } from '@hubtask/design-system/components';

  import FailureNote from '../frame/FailureNote.svelte';
  import type { ConfigTemplate, ConfigTemplateList, ResourceState, TransportError } from '@hubtask/sync-engine';

  import { announcer } from '../announce.svelte.ts';
  import { config } from '../data/config.svelte.ts';
  import { answersOf, offersSamples } from '../data/configtemplates.ts';
  import { workspace } from '../data/workspace.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { onboarding } from '../onboarding/onboarding.svelte.ts';
  import { tour } from '../tour.svelte.ts';
  import { renderProblem } from '../problem.ts';
  import TemplatePreview from '../settings/TemplatePreview.svelte';
  import { familyMark, markOf, switchedOn, type Family } from './hubGallery.ts';
  import TemplateTakeFields from './TemplateTakeFields.svelte';
  import { afterStarter, PURPOSES, purposeStarterBody, starterFamilies, templateStarterBody, type Purpose } from './starterSet.ts';

  interface Props {
    /** Ran after the set was made, with where to open: the template's main product, else the hub. */
    onstarted?: (path: string) => void;
    /**
     * Set to open the dialog once more after a skip (Today's *Choose what you use Hubtask for*,
     * user-experience.md §3.3); the dialog sets it back when it closes.
     */
    isChoosing?: boolean;
  }

  let { onstarted, isChoosing = $bindable(false) }: Props = $props();

  $effect(() => untrack(() => workspace.open()));

  let isOpen = $state(false);
  let isAnswered = $state(false);
  // Plain, not state: the dialog's onClose runs in an effect's teardown, where a state read answers
  // the value from before the change - a skip already sent would look unsent.
  let isClosing = false;
  let isBusy = $state(false);
  let failure = $state<string | undefined>(undefined);

  // Opened once, when the workspace says the question is offered; the tour waits while it is.
  $effect(() => {
    if (workspace.workspace?.starter_offered === true && !isAnswered) isOpen = true;
  });
  // Opened again from Today after a skip: the question is closed already, so a dismiss sends nothing.
  let isReopened = false;
  $effect(() => {
    if (!isChoosing) return;
    untrack(() => {
      isReopened = true;
      isClosing = false;
      isAnswered = false;
      failure = undefined;
      isOpen = true;
    });
  });
  $effect(() => {
    if (!isOpen && isChoosing) untrack(() => (isChoosing = false));
  });
  $effect(() => {
    onboarding.starterOpen(isOpen);
  });

  // The catalogue, once the dialog is open; without it the starter is *Simple* alone.
  let list = $state<ResourceState<ConfigTemplateList>>();
  $effect(() => {
    if (!isOpen) return;
    return untrack(() => config.templates((next) => (list = next)));
  });
  const families = $derived(starterFamilies(list?.status === 'ready' ? list.data.data : []));

  let family = $state<Family | 'simple'>();
  let chosen = $state<ConfigTemplate>();
  let answers = $state<Record<string, string>>({});
  let withSamples = $state(true);
  let acknowledged = $state(false);
  let hubName = $state('');

  const familyTemplates = $derived(families.find((each) => each.family === family)?.templates ?? []);
  const samplesOffered = $derived(offersSamples(chosen, 'starter'));

  const nameOf = (template: ConfigTemplate) =>
    template.origin === 'HUBTASK' ? t(`app.structure.preset.${template.key.toUpperCase()}`) : template.name;
  const summaryOf = (template: ConfigTemplate) =>
    template.origin === 'HUBTASK' ? t(`app.templates.summary.${template.key.toUpperCase()}`) : undefined;
  const productsOf = (template: ConfigTemplate): string[] =>
    switchedOn(template).map((module) => t(`app.modules.name_${module.toLowerCase()}`));
  const familyAbout = (each: Family | 'simple') =>
    messages.has(`app.starter.family_about.${each}`) ? t(`app.starter.family_about.${each}`) : undefined;

  let isPlanning = $state(false);
  let mustAcknowledge = $state(false);

  function pickTemplate(template: ConfigTemplate) {
    chosen = template;
    answers = {};
    withSamples = true;
    acknowledged = false;
    hubName = nameOf(template);
  }

  function back() {
    failure = undefined;
    if (chosen) {
      chosen = undefined;
    } else {
      family = undefined;
    }
  }

  async function skip() {
    if (isClosing) return;
    isClosing = true;
    isAnswered = true;
    isOpen = false;
    if (isReopened) return;
    try {
      await workspace.closeStarter();
    } catch {
      // Asked again on the next visit: nothing was made.
      isClosing = false;
      isAnswered = false;
    }
  }

  // One key per answer, kept across retries: a repeat after a lost answer replays the set.
  const keys: Record<string, string> = {};

  async function run(answerKey: string, body: Parameters<typeof workspace.applyStarter>[0], template?: ConfigTemplate) {
    if (isBusy) return;
    isBusy = true;
    failure = undefined;
    keys[answerKey] ??= crypto.randomUUID();
    try {
      const hub = await workspace.applyStarter(body, keys[answerKey]);
      tour.starterMade(hub.id);
      isClosing = true;
      isAnswered = true;
      announcer.say(t('app.starter.done'));
      isOpen = false;
      onstarted?.(afterStarter(template, hub.id));
    } catch (error) {
      failure = renderProblem(error as TransportError, messages).message;
    } finally {
      isBusy = false;
    }
  }

  function startPurpose(purpose: Purpose) {
    void run(`purpose:${purpose}`, purposeStarterBody(purpose, (code) => t(code)));
  }

  function startTemplate() {
    if (!chosen) return;
    const name = hubName.trim() || nameOf(chosen);
    // The key covers the answers: another answer is another set, a retry of the same one replays.
    const given = answersOf(chosen.questions, answers);
    void run(`template:${chosen.key}:${JSON.stringify(given)}:${withSamples}`,
      templateStarterBody(name, chosen.key, given, samplesOffered && withSamples, acknowledged), chosen);
  }

</script>

<!-- Dismissed without an answer is a skip: the question is not asked again. -->
<Dialog bind:isOpen title={chosen ? nameOf(chosen) : t('app.starter.title')} dismissLabel={t('app.starter.skip')} onClose={() => void skip()}>
  <div data-starter data-starter-step={chosen ? 'template' : family ? 'gallery' : 'families'}>
    <Stack gap="150">
      {#if !family}
        <p class="hint">{t('app.starter.about')}</p>
        <TemplateGallery label={t('app.starter.families')}>
          {#each families as each (each.family)}
            {@const label = t(`app.workspace.hub_family.${each.family}`)}
            <TemplateCard name={label} mark={each.family === 'simple' ? 'sparkles' : familyMark(each.family)} summary={familyAbout(each.family)} data-starter-family={each.family}>
              {#snippet action()}
                <Button size="sm" tone="secondary" onclick={() => (family = each.family)}>
                  {t('app.starter.choose')}<VisuallyHidden>{` ${label}`}</VisuallyHidden>
                </Button>
              {/snippet}
            </TemplateCard>
          {/each}
        </TemplateGallery>
        {#if list?.status === 'loading'}<Spinner label={t('app.templates.reading')} />{/if}
      {:else if family === 'simple'}
        <p class="hint" data-starter-samples-note>{t('app.starter.simple_samples')}</p>
        <TemplateGallery label={t('app.workspace.hub_family.simple')}>
          {#each PURPOSES as purpose (purpose)}
            {@const label = t(`app.starter.purpose.${purpose}`)}
            <TemplateCard name={label} summary={t(`app.starter.purpose_about.${purpose}`)} isCompact data-starter-purpose={purpose}>
              {#snippet action()}
                <Button size="sm" tone="primary" isBusy={isBusy} busyLabel={t('app.starter.starting')} onclick={() => startPurpose(purpose)}>
                  {t('app.starter.start')}<VisuallyHidden>{` ${label}`}</VisuallyHidden>
                </Button>
              {/snippet}
            </TemplateCard>
          {/each}
        </TemplateGallery>
      {:else if !chosen}
        <TemplateGallery label={t(`app.workspace.hub_family.${family}`)}>
          {#each familyTemplates as each (each.key)}
            {@const name = nameOf(each)}
            <TemplateCard {name} mark={markOf(each)} summary={summaryOf(each)} products={productsOf(each)} isCompact data-starter-template={each.key}>
              {#snippet details()}<TemplatePreview template={each} />{/snippet}
              {#snippet action()}
                <Button size="sm" tone="secondary" onclick={() => pickTemplate(each)}>
                  {t('app.starter.choose')}<VisuallyHidden>{` ${name}`}</VisuallyHidden>
                </Button>
              {/snippet}
            </TemplateCard>
          {/each}
        </TemplateGallery>
      {:else}
        {#if summaryOf(chosen)}<p class="hint">{summaryOf(chosen)}</p>{/if}
        <Input label={t('app.starter.hub_name')} value={hubName} data-starter-hub-name
          oninput={(event: Event) => (hubName = (event.currentTarget as HTMLInputElement).value)} />
        <TemplateTakeFields template={chosen} plansWorkspace {samplesOffered} bind:answers bind:withSamples bind:acknowledged
          bind:isPlanning bind:mustAcknowledge bind:failure />
      {/if}
      <p class="hint">{t('app.starter.changeable')}</p>
      {#if failure}<FailureNote problem={failure} />{/if}
    </Stack>
  </div>

  {#snippet actions()}
    {#if family}
      <Button tone="secondary" onclick={back}>{t('app.starter.back')}</Button>
    {:else}
      <Button tone="secondary" onclick={() => void skip()}>{t('app.starter.skip')}</Button>
    {/if}
    {#if chosen}
      <Button tone="primary" isBusy={isBusy} busyLabel={t('app.starter.starting')} data-starter-start
        disabledReason={isPlanning ? t('app.starter.plan_reading') : mustAcknowledge && !acknowledged ? t('app.changes.acknowledge_first') : undefined}
        onclick={startTemplate}>{t('app.starter.start')}</Button>
    {/if}
  {/snippet}
</Dialog>

<style>
  .hint { margin: 0; color: var(--text-subtle); font-size: var(--fs-075); max-width: 64ch; }
</style>
