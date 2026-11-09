<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The third-party apps this workspace has registered, as *Machines and apps*' second list page
  // (page-system §7.1). A row opens the app's record page.
  //
  // **The redirect URIs are matched byte for byte**, at authorization and again at exchange. So the
  // form says that, and this screen normalises nothing: a trailing slash quietly removed here is an
  // app that fails to match its own address, and the failure appears much later and somewhere else.
  //
  // **Confidential or public is the app's shape, not a preference.** An app that can keep a secret
  // on a server is confidential and gets one; a native or single-page app cannot keep one and
  // brings PKCE instead. Getting it wrong is how a secret ends up in a bundle somebody can read.
  //
  // **Registering is the head's primary**, a Dialog M (three fields); the secret it answers is shown
  // once in a Dialog S, and removing is in the row's menu behind a Dialog S that says what it costs.

  import { untrack } from 'svelte';

  import {
    Badge,
    Button,
    Checkbox,
    DataTable,
    Dialog,
    EmptyState,
    FilterBar,
    Input,
    ListPage,
    Radio,
    Spinner,
    Stack,
    Textarea,
    type DataColumn,
    type DataRowMenu,
    type Filter,
  } from '@hubtask/design-system/components';

  import { apps, type App, type RegisteredApp } from '../lib/data/apps.svelte.ts';
  import { mayHere } from '../lib/data/capability.svelte.ts';
  import { people } from '../lib/data/people.svelte.ts';
  import { serviceAccounts } from '../lib/data/serviceaccounts.svelte.ts';
  import { announcer } from '../lib/announce.svelte.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { renderProblem } from '../lib/problem.ts';
  import { isComplete, type Missing } from '../lib/forms/missing.ts';
  import { marked } from '../lib/forms/press.ts';
  import { page } from '../lib/frame/page.svelte.ts';
  import FailureNote from '../lib/frame/FailureNote.svelte';
  import AppSecretDialog from '../lib/machines/AppSecretDialog.svelte';
  import MachinesFigure from '../lib/machines/MachinesFigure.svelte';
  import MachinesStrip from '../lib/machines/MachinesStrip.svelte';
  import RemoveAppDialog from '../lib/machines/RemoveAppDialog.svelte';
  import { machinesHead } from '../lib/machines/machineshead.svelte.ts';
  import DiscardDialog from '../lib/page/DiscardDialog.svelte';
  import { DiscardGuard } from '../lib/page/discard.svelte.ts';
  import { matchesChoice, matchesSearch } from '../lib/page/list.ts';
  import { sorting } from '../lib/sorting.svelte.ts';
  import WorkspaceGate from '../lib/workspace/WorkspaceGate.svelte';

  const { onnavigate }: { onnavigate?: (path: string) => void } = $props();

  const TENANT = { scopeType: 'TENANT' } as const;

  $effect(() =>
    untrack(() => {
      // The service accounts and their memberships are counted in the head's figure.
      people.openScope(TENANT);
      const stopAccounts = serviceAccounts.open();
      const stopApps = apps.open();
      return () => {
        stopAccounts();
        stopApps();
      };
    }),
  );
  $effect(() => page.entitle(t('app.machines.title')));

  let name = $state('');
  /** What *Register* found missing when pressed, marked at the fields (UC-WRK-32 check 3). */
  let missing = $state<Missing>({});
  let uris = $state('');
  let confidential = $state(false);
  /** What registering answered, until its dialog closes; the secret dies with it. */
  let registered = $state<RegisteredApp | undefined>(undefined);
  let removing = $state<App | undefined>(undefined);
  let addFailure = $state<string | undefined>(undefined);
  let isWorking = $state(false);
  let isAdding = $state(false);
  let addForm = $state<HTMLFormElement | undefined>(undefined);

  // The secret dies with the screen, like every other one-time value in this client.
  $effect(() => () => (registered = undefined));

  const reading = $derived(apps.state);
  const refusal = $derived(reading.status === 'failed' ? renderProblem(reading.error, messages) : undefined);

  /** One per line, and nothing else touched: they are matched byte for byte. */
  const readUris = (written: string) => written.split('\n').map((one) => one.trim()).filter((one) => one !== '');

  function reset(): void {
    name = '';
    uris = '';
    confidential = false;
    missing = {};
    addFailure = undefined;
  }

  const addGuard = new DiscardGuard(() => name.trim() !== '' || uris.trim() !== '' || confidential, () => {
    isAdding = false;
    reset();
  });

  async function register(event: SubmitEvent): Promise<void> {
    event.preventDefault();
    const list = readUris(uris);
    missing = marked(event, [
      { kind: 'field', key: 'name', code: 'app.forms.required', isFilled: name.trim() !== '' },
      { kind: 'field', key: 'uris', code: 'app.forms.required', isFilled: list.length > 0 },
    ]);
    if (!isComplete(missing)) return;
    addFailure = undefined;
    isWorking = true;
    try {
      const answer = await apps.register(name.trim(), list, confidential);
      isAdding = false;
      reset();
      registered = answer;
      announcer.say(t('app.apps.registered_announced'));
    } catch (error) {
      addFailure = renderProblem(error as never, messages).message;
    } finally {
      isWorking = false;
    }
  }

  /** Whether the reader may register and remove apps (`MANAGE_MEMBERS`), as the server asks. */
  const mayManage = $derived(mayHere('MANAGE_MEMBERS').status === 'permitted');

  // The search and the kind narrow the apps loaded: `GET /oauth/clients` answers them whole.
  let search = $state('');
  let kind = $state('');
  const shown = $derived(
    apps.all.filter(
      (app) => matchesSearch(search, [app.name, ...app.redirect_uris]) && matchesChoice(kind || undefined, app.confidential ? 'confidential' : 'public'),
    ),
  );
  const chips = $derived<Filter[]>([
    { id: 'kind', label: t('app.apps.kind_chip', { kind: kind ? t(`app.apps.${kind}`) : t('app.page.any') }), isActive: kind !== '' },
  ]);

  const columns = $derived<DataColumn[]>([
    { id: 'name', label: t('app.apps.name'), isSortable: true, priority: 1 },
    { id: 'uris', label: t('app.apps.uris'), priority: 2 },
    { id: 'kind', label: t('app.apps.kind_column'), isStatus: true },
  ]);
  const order = sorting({ rows: () => shown, read: (app: App) => app.name });

  function menuOf(app: App): DataRowMenu | undefined {
    if (!mayManage) return undefined;
    return { label: t('app.page.more_for', { name: app.name }), items: [{ id: 'remove', label: t('app.apps.remove_menu'), isDestructive: true }] };
  }

  const primary = $derived(
    mayManage ? { label: t('app.apps.new_title'), icon: 'plus' as const, opener: 'register-app', onclick: () => (isAdding = true) } : undefined,
  );

  function clearFilters(): void {
    search = '';
    kind = '';
  }
</script>

{#snippet figure()}<MachinesFigure />{/snippet}

<ListPage head={machinesHead(primary, figure)}>
  {#snippet strip()}<MachinesStrip current="apps" {onnavigate} />{/snippet}
  {#snippet intro()}<p class="quiet">{t('app.apps.intro')}</p>{/snippet}
  {#snippet filters()}
    <FilterBar
      searchLabel={t('app.apps.search')}
      clearSearchLabel={t('app.page.clear_search')}
      bind:search
      filters={chips}
      moreLabel={t('app.page.more_filters')}
      filtersLabel={t('app.page.filters', { count: String(kind ? 1 : 0) })}
      sheetTitle={t('app.page.filters', { count: '0' })}
      sheetDismissLabel={t('app.page.sheet_close')}
      sheetGrabLabel={t('app.page.sheet_grab')}
      showLabel={t('app.page.show_results', { count: String(shown.length) })}
      clearLabel={t('app.page.clear')}
      onclear={clearFilters}
      countLabel={reading.status === 'ready' ? t('app.page.found', { shown: String(shown.length), count: String(apps.all.length) }) : undefined}
    >
      {#snippet picker(_filter: Filter)}
        <Radio
          label={t('app.apps.kind_column')}
          bind:value={kind}
          options={[
            { value: '', label: t('app.page.any') },
            { value: 'confidential', label: t('app.apps.confidential') },
            { value: 'public', label: t('app.apps.public') },
          ]}
        />
      {/snippet}
    </FilterBar>
  {/snippet}

  <!-- A reader without MANAGE_MEMBERS is told once why nothing here can be registered (P-05). -->
  <WorkspaceGate permission="MANAGE_MEMBERS" />

  {#if reading.status === 'loading' || reading.status === 'idle'}
    <p class="quiet waiting"><Spinner label={t('app.apps.reading')} /> <span>{t('app.apps.reading')}</span></p>
  {:else if refusal}
    <FailureNote problem={refusal} />
  {:else}
    <DataTable
      label={t('app.apps.title')}
      {columns}
      rows={order.rows}
      key={(app: App) => app.id}
      href={(app: App) => `/administration/apps/${app.id}`}
      rowLabel={(app: App) => app.name}
      menu={menuOf}
      onmenu={(app: App) => (removing = app)}
      actionsLabel={t('app.page.row_actions')}
      sheetDismissLabel={t('app.page.sheet_close')}
      sheetGrabLabel={t('app.page.sheet_grab')}
      sort={order.sort}
      onSort={(next) => order.by(next)}
      shownLabel={apps.all.length === 0 ? undefined : t('app.page.showing', { count: String(shown.length) })}
    >
      {#snippet cell(app: App, column: DataColumn)}
        {#if column.id === 'name'}
          <span class="name">{app.name}</span>
        {:else if column.id === 'uris'}
          <span class="uris">{app.redirect_uris.join(', ')}</span>
        {:else}
          <Badge tone={app.confidential ? 'info' : 'neutral'}>{app.confidential ? t('app.apps.confidential') : t('app.apps.public')}</Badge>
        {/if}
      {/snippet}
      {#snippet empty()}
        {#if apps.all.length === 0}
          <EmptyState kind="unused" icon="key" title={t('app.apps.none')} description={t('app.apps.empty_body')} action={mayManage ? addAct : undefined} />
        {:else}
          <EmptyState kind="filtered" title={t('app.apps.filtered_title')}>
            {#snippet action()}<Button onclick={clearFilters}>{t('app.people.filtered_clear')}</Button>{/snippet}
          </EmptyState>
        {/if}
      {/snippet}
    </DataTable>
  {/if}
</ListPage>

{#snippet addAct()}
  <Button tone="primary" icon="plus" onclick={() => (isAdding = true)}>{t('app.apps.new_title')}</Button>
{/snippet}

{#if mayManage}
  <Dialog
    size="m"
    bind:isOpen={isAdding}
    title={t('app.apps.new_title')}
    dismissLabel={t('app.workspace.cancel')}
    onDismiss={addGuard.dismiss}
    onClose={() => (addFailure = undefined)}
    primary={{ label: t('app.apps.register'), isBusy: isWorking, busyLabel: t('app.apps.registering'), onclick: () => addForm?.requestSubmit() }}
  >
    <form bind:this={addForm} onsubmit={register}>
      <Stack gap="200">
        {#if addFailure}<FailureNote problem={addFailure} />{/if}
        <Input label={t('app.apps.name')} hint={t('app.apps.name_hint')} bind:value={name} isRequired error={missing.name && !name.trim() ? t(missing.name) : undefined} />
        <Textarea
          label={t('app.apps.uris')}
          hint={t('app.apps.uris_hint')}
          bind:value={uris}
          error={missing.uris && readUris(uris).length === 0 ? t(missing.uris) : undefined}
          rows={3}
          spellcheck={false}
        />
        <Checkbox
          label={t('app.apps.is_confidential')}
          hint={t('app.apps.is_confidential_hint')}
          checked={confidential}
          onchange={(event: Event) => (confidential = (event.currentTarget as HTMLInputElement).checked)}
        />
      </Stack>
    </form>
    {#snippet actions()}
      <Button tone="subtle" onclick={() => { if (addGuard.dismiss()) { isAdding = false; reset(); } }}>{t('app.workspace.cancel')}</Button>
    {/snippet}
  </Dialog>
  <DiscardDialog guard={addGuard} />
{/if}

<AppSecretDialog {registered} onClose={() => (registered = undefined)} />
<RemoveAppDialog app={removing} onClose={() => (removing = undefined)} />

<style>
  .quiet { margin: 0; color: var(--text-secondary); }
  .waiting { display: flex; align-items: center; gap: var(--sp-100); }
  .name { color: var(--text-primary); font-weight: var(--fw-medium); }
  .uris { font-family: var(--font-mono); font-size: var(--fs-075); overflow-wrap: anywhere; color: var(--text-secondary); }
  form { margin: 0; }
</style>
