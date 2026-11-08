<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import { platform } from '../lib/platform/index.ts';
  // Voice assistants (ADR-0093, UC-WRK-35): a workspace's administrator switches spoken entries on,
  // picks the list a thing said without a place goes to, names the places by what people say, tries
  // a sentence, and connects Alexa.
  //
  // **Connecting Alexa is copying.** Every value the Alexa developer console asks for stands here
  // with its own copy button, in the console's order: the skill, the model, the endpoint, the
  // account linking, the skill's ID. The one value made here - the linking's client - is made with
  // one press from the vendor ID, and its secret is shown once.
  //
  // **Where the installation publishes a skill**, all of that is the operator's: the page says to
  // enable it in the Alexa app and link the account.
  //
  // **The head says what is connected** (user-experience.md §7.10), from what is saved rather
  // than the draft: spoken entries on or off, and whether a skill answers. Nothing here fails on
  // its own - a skill that stops answering is Alexa's to say - so nothing is counted in the column.
  //
  // **One section per address, one save each** (page-system §7.3, PS-9, PS-10): *Spoken entries*,
  // *Names people say*, *Try a sentence* and *Connect Alexa*, each at
  // `/administration/voice/<section>`. The settings are one record written whole, so a section's
  // save bar sends its own values over what is saved and leaves another section's draft where it is.

  import { untrack } from 'svelte';

  import FailureNote from '../lib/frame/FailureNote.svelte';

  import {
    Badge,
    Button,
    IconButton,
    Input,
    OneTimeSecret,
    SaveBar,
    Select,
    SettingRow,
    SettingsLayout,
    SettingsSection,
    Spinner,
    Switch,
    type SectionLink,
  } from '@hubtask/design-system/components';

  import { announcer } from '../lib/announce.svelte.ts';
  import { apps, type RegisteredApp } from '../lib/data/apps.svelte.ts';
  import { containers } from '../lib/data/containers.svelte.ts';
  import { labels } from '../lib/data/labels.svelte.ts';
  import {
    ALEXA_SCOPES,
    alexaRedirects,
    readAlexaModel,
    readVoiceSettings,
    saveVoiceSettings,
    tryVoiceSentence,
    type VoiceInterpretation,
    type VoiceSettings,
  } from '../lib/data/voice.svelte.ts';
  import { adminTrail } from '../lib/administration/trail.ts';
  import { leaveGuard } from '../lib/frame/leave.svelte.ts';
  import { page } from '../lib/frame/page.svelte.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { renderProblem } from '../lib/problem.ts';
  import WorkspaceGate from '../lib/workspace/WorkspaceGate.svelte';
  import { holdsHere } from '../lib/data/capability.svelte.ts';

  interface Props {
    /** The section the address names (`/administration/voice/:section`); none is the list. */
    section?: string;
    /** Replaces the address: a move between sections, so the unsaved values of another stay. */
    onreplace?: (path: string) => void;
  }

  const { section, onreplace }: Props = $props();

  const BASE = '/administration/voice';
  type Scope = 'on' | 'names' | 'alexa';

  interface DraftAlias {
    key: string;
    phrase: string;
    collection: string;
    label: string;
  }

  let loaded = $state<VoiceSettings | undefined>(undefined);
  const isSkillConnected = $derived(loaded?.installation_skill === true || Boolean(loaded?.skill_id));
  let readFailure = $state<string | undefined>(undefined);
  let enabled = $state(false);
  let defaultList = $state('');
  let skillId = $state('');
  let invocation = $state('');
  let aliases = $state<DraftAlias[]>([]);
  let isSaving = $state(false);
  let saveFailure = $state<string | undefined>(undefined);

  // Read only for whom the page is: anybody else is told by the gate below, and a read they may not
  // make would only be refused.
  const mayShape = $derived(holdsHere('AUTOMATION') === true);
  $effect(() => {
    if (!mayShape) return;
    untrack(() => {
      void readVoiceSettings()
        .then((settings) => settings && take(settings))
        .catch((error) => (readFailure = renderProblem(error as never, messages).message));
    });
  });

  /** What is saved becomes the draft of the scopes named: all on a read, one on a section's save. */
  function take(settings: VoiceSettings, scopes: readonly Scope[] = ['on', 'names', 'alexa']) {
    loaded = { ...settings, aliases: settings.aliases ?? [] };
    if (scopes.includes('on')) {
      enabled = settings.enabled === true;
      defaultList = settings.default_collection_id ?? '';
    }
    if (scopes.includes('alexa')) {
      skillId = settings.skill_id ?? '';
      invocation = settings.invocation_name ?? '';
    }
    if (scopes.includes('names')) {
      aliases = (settings.aliases ?? []).map((alias) => ({
        key: crypto.randomUUID(), phrase: alias.phrase, collection: alias.collection_id, label: alias.label_id ?? '',
      }));
    }
  }

  const aliasesOf = (rows: readonly { phrase: string; collection: string; label: string }[]) =>
    rows.filter((row) => row.phrase.trim() !== '').map((row) => `${row.phrase.trim()}\u0000${row.collection}\u0000${row.label}`).join('\n');

  /** How many values of each section differ from what is saved: its save bar's count. */
  const changed = $derived.by((): Record<Scope, number> => {
    const held = loaded;
    if (!held) return { on: 0, names: 0, alexa: 0 };
    const count = (flags: boolean[]) => flags.filter(Boolean).length;
    return {
      on: count([enabled !== (held.enabled === true), defaultList !== (held.default_collection_id ?? '')]),
      names: count([aliasesOf(aliases) !== aliasesOf((held.aliases ?? []).map((alias) => ({ phrase: alias.phrase, collection: alias.collection_id, label: alias.label_id ?? '' })))]),
      alexa: count([skillId.trim() !== (held.skill_id ?? ''), invocation.trim().toLowerCase() !== (held.invocation_name ?? '')]),
    };
  });
  /** Every section's unsaved values: what the leave guard holds, as a move between sections keeps them. */
  $effect(() => leaveGuard.hold(() => changed.on + changed.names + changed.alexa));

  const SECTIONS = [
    { id: 'on', code: 'app.voice.section_on' },
    { id: 'names', code: 'app.voice.section_names' },
    { id: 'try', code: 'app.voice.section_try' },
    { id: 'alexa', code: 'app.voice.section_alexa' },
  ] as const;
  const isKnown = (id: string | undefined): boolean => id !== undefined && SECTIONS.some((each) => each.id === id);
  const shown = $derived(isKnown(section) ? (section as string) : 'on');
  const view = $derived<'list' | 'detail'>(isKnown(section) ? 'detail' : 'list');

  function statusOf(id: string): Pick<SectionLink, 'status' | 'statusTone'> {
    const held = loaded;
    if (!held) return {};
    if (id === 'on') return held.enabled ? { status: t('app.voice.status_on'), statusTone: 'set' } : { status: t('app.voice.status_off') };
    if (id === 'names') {
      const count = held.aliases?.length ?? 0;
      return count > 0 ? { status: t('app.voice.status_names', { count: String(count) }), statusTone: 'set' } : { status: t('app.voice.status_none') };
    }
    if (id === 'alexa') return isSkillConnected ? { status: t('app.voice.status_connected'), statusTone: 'set' } : { status: t('app.voice.status_not_connected') };
    return {};
  }

  const title = $derived(t('app.voice.title'));
  const links = $derived<SectionLink[]>(SECTIONS.map((each) => ({ id: each.id, label: t(each.code), href: `${BASE}/${each.id}`, ...statusOf(each.id) })));
  $effect(() => page.entitle(title));

  // The tree is not drawn in the administration, so the lists are read here.
  $effect(() => untrack(() => containers.start()));
  $effect(() => {
    const stops = containers.hubs.map((hub) => untrack(() => containers.openLevel(hub.id)));
    return () => stops.forEach((stop) => stop());
  });

  /** Every collection the reader sees, by hub, for the list pickers. */
  const collections = $derived(
    containers.hubs.flatMap((hub) =>
      containers.collectionsOf(hub.id).map((collection) => ({ value: collection.id, label: `${hub.name} · ${collection.name}` })),
    ),
  );

  // The categories of each collection an alias points at, for its second picker.
  $effect(() => {
    const wanted = [...new Set(aliases.map((alias) => alias.collection).filter(Boolean))];
    const closers = untrack(() => wanted.map((id) => labels.open(id)));
    return () => closers.forEach((close) => close());
  });
  const categoriesOf = (collection: string) =>
    labels.of(collection).map((label) => ({ value: label.id, label: label.name }));

  function addAlias() {
    aliases = [...aliases, { key: crypto.randomUUID(), phrase: '', collection: defaultList, label: '' }];
  }

  function removeAlias(key: string) {
    aliases = aliases.filter((alias) => alias.key !== key);
  }

  /** Writes one section's values over what is saved; the other sections' drafts stay as they are. */
  async function save(scope: Scope) {
    const held = loaded;
    if (!held) return;
    saveFailure = undefined;
    isSaving = true;
    const own = (name: Scope) => scope === name;
    try {
      take(await saveVoiceSettings({
        enabled: own('on') ? enabled : held.enabled === true,
        default_collection_id: (own('on') ? defaultList : held.default_collection_id) || null,
        skill_id: (own('alexa') ? skillId.trim() : held.skill_id) || null,
        invocation_name: (own('alexa') ? invocation.trim().toLowerCase() : held.invocation_name) || null,
        aliases: own('names')
          ? aliases
            .filter((alias) => alias.phrase.trim() !== '')
            .map((alias) => ({ phrase: alias.phrase.trim(), collection_id: alias.collection, ...(alias.label ? { label_id: alias.label } : {}) }))
          : (held.aliases ?? []),
      }), [scope]);
      announcer.say(t('app.voice.saved'));
    } catch (error) {
      saveFailure = renderProblem(error as never, messages).message;
    } finally {
      isSaving = false;
    }
  }

  // *Try a sentence*: what Alexa would do, without doing it.
  // The interface's language until somebody chooses another: read when the catalogue has loaded,
  // not when the page was first drawn.
  const spoken = $derived(messages.locale.startsWith('de') ? 'de-DE' : 'en-US');
  /** The name the skill is spoken to by where none was chosen: two words Alexa writes apart. */
  const defaultInvocation = $derived(spoken === 'de-DE' ? 'digitale pinnwand' : 'list helper');
  /** The name in force: the one saved, else the default - what the console and people say. */
  const spokenName = $derived(loaded?.invocation_name || defaultInvocation);
  let chosenLocale = $state<string | undefined>(undefined);
  const locale = $derived(chosenLocale ?? spoken);
  let typed = $state<string | undefined>(undefined);
  const sentence = $derived(typed ?? t('app.voice.try_example'));
  let tried = $state<VoiceInterpretation | undefined>(undefined);
  let tryFailure = $state<string | undefined>(undefined);
  async function tryIt() {
    tryFailure = undefined;
    try {
      tried = await tryVoiceSentence(sentence, locale);
    } catch (error) {
      tryFailure = renderProblem(error as never, messages).message;
    }
  }
  const triedThings = $derived(
    tried ? tried.things.map((thing) => (thing.amount ? `${thing.amount} ${thing.title}` : thing.title)).join(', ') : '',
  );

  // Connecting Alexa.
  const origin = platform.webOrigin;
  const endpoint = `${origin}/api/v1/voice/alexa`;
  const authorizationUri = `${origin}/oauth/consent`;
  const tokenUri = `${origin}/api/v1/oauth/token`;
  let vendorId = $state('');
  let registered = $state<RegisteredApp | undefined>(undefined);
  let linkFailure = $state<string | undefined>(undefined);
  let isLinking = $state(false);
  $effect(() => () => (registered = undefined));

  async function makeLinking() {
    linkFailure = undefined;
    isLinking = true;
    try {
      registered = await apps.register('Alexa', alexaRedirects(vendorId), true);
      announcer.say(t('app.voice.linking_made'));
    } catch (error) {
      linkFailure = renderProblem(error as never, messages).message;
    } finally {
      isLinking = false;
    }
  }

  let copied = $state<string | undefined>(undefined);
  async function copy(id: string, value: string) {
    try {
      await navigator.clipboard.writeText(value);
      copied = id;
      announcer.say(t('app.voice.copied'));
    } catch {
      copied = undefined;
    }
  }

  let chosenModelLocale = $state<string | undefined>(undefined);
  const modelLocale = $derived(chosenModelLocale ?? spoken);
  let modelFailure = $state<string | undefined>(undefined);
  async function copyModel() {
    modelFailure = undefined;
    try {
      const model = await readAlexaModel(modelLocale);
      await copy('model', JSON.stringify(model, null, 2));
    } catch (error) {
      modelFailure = renderProblem(error as never, messages).message;
    }
  }

  const locales = [
    { value: 'de-DE', label: 'Deutsch (de-DE)' },
    { value: 'en-US', label: 'English (en-US)' },
    { value: 'en-GB', label: 'English (en-GB)' },
  ];
</script>

{#snippet value(id: string, label: string, text: string)}
  <div class="value" data-copy={id}>
    <span class="value-label">{label}</span>
    <code>{text}</code>
    <IconButton icon={copied === id ? 'check' : 'copy'} size="sm" label={t('app.voice.copy_named', { name: label })} onclick={() => void copy(id, text)} />
  </div>
{/snippet}

{#snippet saveBar(scope: Scope)}
  <SaveBar
    label={t('app.save_bar.label')}
    changesLabel={changed[scope] > 0 ? t('app.save_bar.changes', { count: String(changed[scope]) }) : undefined}
    discardLabel={t('app.save_bar.discard')}
    saveLabel={t('app.voice.save')}
    {isSaving}
    savingLabel={t('app.voice.saving')}
    onDiscard={() => {
      saveFailure = undefined;
      if (loaded) take(loaded, [scope]);
    }}
    onSave={() => void save(scope)}
  />
{/snippet}
{#snippet onBar()}{@render saveBar('on')}{/snippet}
{#snippet namesBar()}{@render saveBar('names')}{/snippet}
{#snippet alexaBar()}{@render saveBar('alexa')}{/snippet}

{#snippet figure()}
  {#if loaded}
    <span class="figure" data-voice-figure>
      <Badge tone={loaded.enabled ? 'success' : 'neutral'}>{t(loaded.enabled ? 'app.voice.state_on' : 'app.voice.state_off')}</Badge>
      <Badge tone={isSkillConnected ? 'success' : 'neutral'}>{t(isSkillConnected ? 'app.voice.state_skill' : 'app.voice.state_no_skill')}</Badge>
    </span>
  {/if}
{/snippet}

<SettingsLayout
  head={{ title, subtitle: t('app.voice.intro'), breadcrumb: adminTrail({ id: 'voice', label: title }), figure }}
  nav={{ label: t('app.settings_page.sections', { name: title }), sections: links, current: shown, onselect: (next) => onreplace?.(`${BASE}/${next}`) }}
  {view}
  back={{ label: title, onback: () => onreplace?.(BASE) }}
>
  <WorkspaceGate permission="AUTOMATION">
    {#snippet actions()}
      {#if readFailure}
        <FailureNote problem={readFailure} />
      {:else if !loaded}
        <p class="quiet"><Spinner label={t('app.voice.reading')} /> {t('app.voice.reading')}</p>
      {:else}
        {#if saveFailure}<FailureNote problem={saveFailure} />{/if}
        {#if shown === 'on'}
          <div data-voice-settings>
            <SettingsSection title={t('app.voice.section_on')} save={onBar}>
              <SettingRow label={t('app.voice.enabled')} help={t('app.voice.enabled_hint')} fieldWidth="xs" isChanged={enabled !== (loaded.enabled === true)}>
                {#snippet control()}<Switch label={t('app.voice.enabled')} isLabelHidden bind:checked={enabled} />{/snippet}
              </SettingRow>
              <SettingRow label={t('app.voice.default_list')} help={t('app.voice.default_list_hint')} fieldWidth="md" isChanged={defaultList !== (loaded.default_collection_id ?? '')}>
                {#snippet control(nameId)}
                  <Select label={t('app.voice.default_list')} isLabelHidden aria-labelledby={nameId} placeholder={t('app.voice.no_default')} options={collections} bind:value={defaultList} />
                {/snippet}
              </SettingRow>
            </SettingsSection>
          </div>
        {:else if shown === 'names'}
          <div data-voice-names>
            <SettingsSection title={t('app.voice.section_names')} description={t('app.voice.names_hint')} save={namesBar}>
              <div class="rows">
                {#each aliases as alias (alias.key)}
                  <div class="alias" data-alias>
                    <Input label={t('app.voice.alias_phrase')} bind:value={alias.phrase} />
                    <Select label={t('app.voice.alias_list')} options={collections} bind:value={alias.collection} />
                    <Select label={t('app.voice.alias_category')} placeholder={t('app.voice.alias_whole_list')} options={categoriesOf(alias.collection)} bind:value={alias.label} />
                    <IconButton icon="trash" label={t('app.voice.alias_remove', { name: alias.phrase || '…' })} onclick={() => removeAlias(alias.key)} />
                  </div>
                {/each}
                <div><Button data-list-add tone="secondary" size="sm" icon="plus" onclick={addAlias}>{t('app.voice.alias_add')}</Button></div>
              </div>
            </SettingsSection>
          </div>
        {:else if shown === 'try'}
          <!-- Nothing here is stored: the sentence is tried, not saved (PS-10). -->
          <div data-voice-try>
            <SettingsSection title={t('app.voice.section_try')} description={t('app.voice.try_hint')}>
              <div class="rows">
                <div class="try">
                  <Input label={t('app.voice.try_sentence')} bind:value={() => sentence, (next) => (typed = next)} />
                  <Select label={t('app.voice.try_language')} options={locales} bind:value={() => locale, (next) => (chosenLocale = next)} />
                  <Button tone="secondary" onclick={() => void tryIt()}>{t('app.voice.try')}</Button>
                </div>
                {#if tryFailure}<FailureNote problem={tryFailure} />{/if}
                {#if tried}
                  <p class="result" role="status" data-voice-result>
                    {#if tried.found && tried.place}
                      {t(tried.default ? 'app.voice.result_default' : 'app.voice.result', { things: triedThings, list: tried.place.name })}
                    {:else if tried.ambiguous.length > 0}
                      {t('app.voice.result_ambiguous', { lists: tried.ambiguous.map((place) => place.name).join(', ') })}
                    {:else}
                      {t('app.voice.result_none', { things: triedThings })}
                    {/if}
                  </p>
                {/if}
              </div>
            </SettingsSection>
          </div>
        {:else}
          <div data-voice-alexa>
            <SettingsSection
              title={t('app.voice.section_alexa')}
              description={loaded.installation_skill ? undefined : t('app.voice.alexa_hint')}
              save={loaded.installation_skill ? undefined : alexaBar}
            >
              {#if loaded.installation_skill}
                <p class="rows">{t('app.voice.installation_skill')}</p>
              {:else}
                <ol class="steps">
                  <li>
                    <h3>{t('app.voice.step_skill')}</h3>
                    <p class="quiet small">{t('app.voice.step_skill_hint')}</p>
                    <div class="field">
                      <Input label={t('app.voice.invocation')} hint={t('app.voice.invocation_hint')} placeholder={defaultInvocation} bind:value={invocation} />
                    </div>
                    {@render value('invocation', t('app.voice.invocation_now'), spokenName)}
                  </li>
                  <li>
                    <h3>{t('app.voice.step_model')}</h3>
                    <p class="quiet small">{t('app.voice.step_model_hint')}</p>
                    <div class="try">
                      <Select label={t('app.voice.try_language')} options={locales} bind:value={() => modelLocale, (next) => (chosenModelLocale = next)} />
                      <Button tone="secondary" icon={copied === 'model' ? 'check' : 'copy'} onclick={() => void copyModel()}>{t('app.voice.copy_model')}</Button>
                    </div>
                    {#if modelFailure}<FailureNote problem={modelFailure} />{/if}
                  </li>
                  <li>
                    <h3>{t('app.voice.step_endpoint')}</h3>
                    <p class="quiet small">{t('app.voice.step_endpoint_hint')}</p>
                    {@render value('endpoint', t('app.voice.endpoint'), endpoint)}
                  </li>
                  <li>
                    <h3>{t('app.voice.step_linking')}</h3>
                    <p class="quiet small">{t('app.voice.step_linking_hint')}</p>
                    <div class="try">
                      <Input label={t('app.voice.vendor_id')} hint={t('app.voice.vendor_id_hint')} bind:value={vendorId} />
                      <Button tone="secondary" isBusy={isLinking} busyLabel={t('app.voice.making_linking')} disabledReason={vendorId.trim() ? undefined : t('app.voice.vendor_first')} onclick={() => void makeLinking()}>{t('app.voice.make_linking')}</Button>
                    </div>
                    {#if linkFailure}<FailureNote problem={linkFailure} />{/if}
                    {@render value('authorization', t('app.voice.authorization_uri'), authorizationUri)}
                    {@render value('token', t('app.voice.token_uri'), tokenUri)}
                    {@render value('scheme', t('app.voice.auth_scheme'), 'HTTP Basic')}
                    {@render value('pkce', t('app.voice.pkce'), 'SHA-256')}
                    {@render value('scope', t('app.voice.scope'), ALEXA_SCOPES)}
                    <p class="quiet small">{t('app.voice.scope_hint')}</p>
                    {#if registered}
                      {@render value('client', t('app.voice.client_id'), registered.id)}
                      {#if registered.client_secret}
                        <OneTimeSecret
                          value={registered.client_secret}
                          label={t('app.voice.client_secret')}
                          hint={t('app.voice.client_secret_hint')}
                          revealLabel={t('app.voice.reveal')}
                          hideLabel={t('app.voice.hide')}
                          copyLabel={t('app.voice.copy')}
                          copiedLabel={t('app.voice.copied')}
                        />
                      {/if}
                    {/if}
                  </li>
                  <li>
                    <h3>{t('app.voice.step_skill_id')}</h3>
                    <p class="quiet small">{t('app.voice.step_skill_id_hint')}</p>
                    <div class="field"><Input label={t('app.voice.skill_id')} bind:value={skillId} /></div>
                  </li>
                  <li>
                    <h3>{t('app.voice.step_test')}</h3>
                    <p class="quiet small">{t('app.voice.step_test_hint', { name: spokenName })}</p>
                  </li>
                </ol>
              {/if}
            </SettingsSection>
          </div>
        {/if}
      {/if}
    {/snippet}
  </WorkspaceGate>
</SettingsLayout>

<style>
  .figure { display: inline-flex; flex-wrap: wrap; gap: var(--sp-050); }

  h3 { margin: 0; font-size: var(--fs-200); font-weight: var(--fw-semibold); }

  .quiet { margin: 0; color: var(--text-secondary); }

  .small { font-size: var(--fs-075); }

  /* A section's own content where it is not setting rows: the region's padding, rows apart. */
  .rows { display: grid; gap: var(--sp-150); margin: 0; padding: var(--sp-200); }

  .field { max-inline-size: calc(var(--sp-1000) * 5); }

  .alias, .try {
    display: flex;
    flex-wrap: wrap;
    align-items: end;
    gap: var(--sp-100);
  }

  .alias > :global(*:not(:last-child)), .try > :global(*:first-child) { flex: 1 1 calc(var(--sp-1000) * 2.5); min-inline-size: 0; }

  .result { margin: 0; padding: var(--sp-100) var(--sp-150); border-radius: var(--r-md); background: var(--bg-recess); }

  .steps { display: grid; gap: var(--sp-300); margin: 0; padding-block: var(--sp-200); padding-inline: var(--sp-500) var(--sp-200); }

  .steps li { display: grid; gap: var(--sp-100); }

  .value {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--sp-100);
    min-inline-size: 0;
  }

  .value-label { flex: 0 0 calc(var(--sp-1000) * 2); color: var(--text-secondary); font-size: var(--fs-075); }

  .value code {
    flex: 1 1 auto;
    min-inline-size: 0;
    overflow-wrap: anywhere;
    font-family: var(--font-mono);
    font-size: var(--fs-075);
    padding: var(--sp-025) var(--sp-100);
    border-radius: var(--r-sm);
    background: var(--bg-recess);
  }
</style>
