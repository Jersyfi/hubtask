<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
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

  import { untrack } from 'svelte';

  import {
    Banner,
    Button,
    IconButton,
    Input,
    OneTimeSecret,
    PageHeader,
    Select,
    Spinner,
    Stack,
    Switch,
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
  import { viewport } from '../lib/frame/viewport.svelte.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { renderProblem } from '../lib/problem.ts';
  import WorkspaceGate from '../lib/workspace/WorkspaceGate.svelte';
  import { holdsHere } from '../lib/data/capability.svelte.ts';

  interface DraftAlias {
    key: string;
    phrase: string;
    collection: string;
    label: string;
  }

  let loaded = $state<VoiceSettings | undefined>(undefined);
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
        .then((settings) => {
          if (!settings) return;
          loaded = { ...settings, aliases: settings.aliases ?? [] };
          enabled = settings.enabled === true;
          defaultList = settings.default_collection_id ?? '';
          skillId = settings.skill_id ?? '';
          invocation = settings.invocation_name ?? '';
          aliases = (settings.aliases ?? []).map((alias) => ({
            key: crypto.randomUUID(), phrase: alias.phrase, collection: alias.collection_id, label: alias.label_id ?? '',
          }));
        })
        .catch((error) => (readFailure = renderProblem(error as never, messages).message));
    });
  });

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

  async function save() {
    saveFailure = undefined;
    isSaving = true;
    try {
      loaded = await saveVoiceSettings({
        enabled,
        default_collection_id: defaultList || null,
        skill_id: skillId.trim() || null,
        invocation_name: invocation.trim().toLowerCase() || null,
        aliases: aliases
          .filter((alias) => alias.phrase.trim() !== '')
          .map((alias) => ({ phrase: alias.phrase.trim(), collection_id: alias.collection, ...(alias.label ? { label_id: alias.label } : {}) })),
      });
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
  const origin = typeof location === 'undefined' ? '' : location.origin;
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

<Stack gap="300">
  <PageHeader
    title={t('app.voice.title')}
    isTitleInBar={viewport.isCompact}
    breadcrumb={{
      trail: [
        { id: 'administration', label: t('app.admin.title'), href: '/administration' },
        { id: 'voice', label: t('app.voice.title') },
      ],
      label: t('app.admin.trail'),
      expandLabel: t('app.admin.expand_trail'),
    }}
  />
  <p class="quiet">{t('app.voice.intro')}</p>

  <WorkspaceGate permission="AUTOMATION">
    {#snippet actions()}
      {#if readFailure}
        <Banner tone="danger">{readFailure}</Banner>
      {:else if !loaded}
        <p class="quiet"><Spinner label={t('app.voice.reading')} /> {t('app.voice.reading')}</p>
      {:else}
        <Stack gap="400">
          <section class="panel" aria-labelledby="voice-on" data-voice-settings>
            <Stack gap="150">
              <h2 class="section" id="voice-on">{t('app.voice.section_on')}</h2>
              <Switch label={t('app.voice.enabled')} hint={t('app.voice.enabled_hint')} bind:checked={enabled} />
              <div class="fields">
                <Select label={t('app.voice.default_list')} hint={t('app.voice.default_list_hint')} placeholder={t('app.voice.no_default')} options={collections} bind:value={defaultList} />
              </div>
            </Stack>
          </section>

          <section class="panel" aria-labelledby="voice-names" data-voice-names>
            <Stack gap="150">
              <h2 class="section" id="voice-names">{t('app.voice.section_names')}</h2>
              <p class="quiet small">{t('app.voice.names_hint')}</p>
              {#each aliases as alias (alias.key)}
                <div class="alias" data-alias>
                  <Input label={t('app.voice.alias_phrase')} bind:value={alias.phrase} />
                  <Select label={t('app.voice.alias_list')} options={collections} bind:value={alias.collection} />
                  <Select label={t('app.voice.alias_category')} placeholder={t('app.voice.alias_whole_list')} options={categoriesOf(alias.collection)} bind:value={alias.label} />
                  <IconButton icon="trash" label={t('app.voice.alias_remove', { name: alias.phrase || '…' })} onclick={() => removeAlias(alias.key)} />
                </div>
              {/each}
              <div><Button tone="secondary" size="sm" icon="plus" onclick={addAlias}>{t('app.voice.alias_add')}</Button></div>
            </Stack>
          </section>

          {#if saveFailure}<Banner tone="danger">{saveFailure}</Banner>{/if}
          <div><Button tone="primary" isBusy={isSaving} busyLabel={t('app.voice.saving')} onclick={() => void save()}>{t('app.voice.save')}</Button></div>

          <section class="panel" aria-labelledby="voice-try" data-voice-try>
            <Stack gap="150">
              <h2 class="section" id="voice-try">{t('app.voice.section_try')}</h2>
              <p class="quiet small">{t('app.voice.try_hint')}</p>
              <div class="try">
                <Input label={t('app.voice.try_sentence')} bind:value={() => sentence, (next) => (typed = next)} />
                <Select label={t('app.voice.try_language')} options={locales} bind:value={() => locale, (next) => (chosenLocale = next)} />
                <Button tone="secondary" onclick={() => void tryIt()}>{t('app.voice.try')}</Button>
              </div>
              {#if tryFailure}<Banner tone="danger">{tryFailure}</Banner>{/if}
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
            </Stack>
          </section>

          <section class="panel" aria-labelledby="voice-alexa" data-voice-alexa>
            <Stack gap="200">
              <h2 class="section" id="voice-alexa">{t('app.voice.section_alexa')}</h2>
              {#if loaded.installation_skill}
                <p>{t('app.voice.installation_skill')}</p>
              {:else}
                <p class="quiet small">{t('app.voice.alexa_hint')}</p>
                <ol class="steps">
                  <li>
                    <h3>{t('app.voice.step_skill')}</h3>
                    <p class="quiet small">{t('app.voice.step_skill_hint')}</p>
                    <div class="try">
                      <Input label={t('app.voice.invocation')} hint={t('app.voice.invocation_hint')} placeholder={defaultInvocation} bind:value={invocation} />
                      <Button tone="secondary" isBusy={isSaving} busyLabel={t('app.voice.saving')} onclick={() => void save()}>{t('app.voice.save')}</Button>
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
                    {#if modelFailure}<Banner tone="danger">{modelFailure}</Banner>{/if}
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
                    {#if linkFailure}<Banner tone="danger">{linkFailure}</Banner>{/if}
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
                    <div class="try">
                      <Input label={t('app.voice.skill_id')} bind:value={skillId} />
                      <Button tone="secondary" isBusy={isSaving} busyLabel={t('app.voice.saving')} onclick={() => void save()}>{t('app.voice.save')}</Button>
                    </div>
                  </li>
                  <li>
                    <h3>{t('app.voice.step_test')}</h3>
                    <p class="quiet small">{t('app.voice.step_test_hint', { name: spokenName })}</p>
                  </li>
                </ol>
              {/if}
            </Stack>
          </section>
        </Stack>
      {/if}
    {/snippet}
  </WorkspaceGate>
</Stack>

<style>
  .section { margin: 0; font-family: var(--font-display); font-size: var(--fs-300); font-weight: var(--fw-semibold); }

  h3 { margin: 0; font-size: var(--fs-200); font-weight: var(--fw-semibold); }

  .quiet { margin: 0; color: var(--text-secondary); }

  .small { font-size: var(--fs-075); }

  .panel {
    padding: var(--sp-200);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-lg);
    background: var(--bg-surface);
  }

  .fields { max-inline-size: calc(var(--sp-1000) * 6); }

  .alias, .try {
    display: flex;
    flex-wrap: wrap;
    align-items: end;
    gap: var(--sp-100);
  }

  .alias > :global(*:not(:last-child)), .try > :global(*:first-child) { flex: 1 1 calc(var(--sp-1000) * 2.5); min-inline-size: 0; }

  .result { margin: 0; padding: var(--sp-100) var(--sp-150); border-radius: var(--r-md); background: var(--bg-recess); }

  .steps { display: grid; gap: var(--sp-300); margin: 0; padding-inline-start: var(--sp-300); }

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
