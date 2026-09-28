<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The installation's workspaces and their lifecycle (SI-17, ADR-0070 §5).
  //
  // **The one legitimate tenant enumerator**, and it lists the `tenant` row and nothing inside it:
  // the slug, the display name, the standing, and — while a deletion request stands — when the
  // grace runs out. There is no way from here into a workspace's contents, because the boundary is
  // a database policy and this screen does not go around it.
  //
  // **Suspending and resuming are here; deleting is not.** A hard delete ends a workspace's own
  // audit chain by design and is irreversible after its grace; it is the one lifecycle act this
  // screen deliberately leaves to `hubctl`, where the person doing it has typed the slug.

  import { untrack } from 'svelte';

  import { Badge, Banner, Button, PageHeader, Spinner, Stack, Table } from '@hubtask/design-system/components';
  import { TransportError } from '@hubtask/sync-engine';

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

  let working = $state<string | undefined>(undefined);
  let failure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);

  $effect(() => {
    const stop = untrack(() => instance.openWorkspaces());
    return stop;
  });

  const reading = $derived(instance.workspacesState);
  const workspaces = $derived(instance.workspaces);
  const unreadable = $derived(
    reading.status === 'failed' ? renderProblem(reading.error, messages) : undefined,
  );

  /** The columns, in the order the rows render their cells. The last carries the controls and is
      announced rather than drawn: a heading over two buttons says nothing a reader needs. */
  const columns = $derived([
    { id: 'workspace', label: t('app.instance.column_workspace') },
    { id: 'state', label: t('app.instance.column_state') },
    { id: 'since', label: t('app.instance.column_since') },
    { id: 'actions', label: t('app.instance.column_actions'), isLabelHidden: true },
  ]);

  async function shift(tenantId: string, action: 'suspend' | 'resume'): Promise<void> {
    working = tenantId;
    failure = undefined;
    try {
      await instance.shift(tenantId, action);
    } catch (cause) {
      failure =
        cause instanceof TransportError
          ? renderProblem(cause, messages)
          : { message: messages.t('errors.internal', {}), fields: new Map(), isServerFault: true };
    } finally {
      working = undefined;
    }
  }

  $effect(() => page.entitle(t('app.instance.workspaces')));
</script>

<Stack gap="300">
  <PageHeader title={t('app.instance.workspaces')} isTitleInBar={viewport.isCompact} />

  <InstanceGate onleave={() => onnavigate('/')}>
    <Stack gap="200">
      <p class="quiet">{t('app.instance.workspaces_intro')}</p>

      {#if failure}
        <Banner tone="danger" title={failure.message}>
          {#if failure.reference}{t('app.error_reference', { request_id: failure.reference })}{/if}
        </Banner>
      {/if}

      {#if reading.status === 'loading' || reading.status === 'idle'}
        <p class="quiet">
          <Spinner label={t('app.instance.reading')} />
          <span>{t('app.instance.reading')}</span>
        </p>
      {:else if unreadable}
        <Banner tone="danger" title={unreadable.message}>
          {#if unreadable.reference}{t('app.error_reference', { request_id: unreadable.reference })}{/if}
        </Banner>
      {:else if workspaces.length === 0}
        <Banner tone="info">{t('app.instance.workspaces_none')}</Banner>
      {:else}
        <Table label={t('app.instance.workspaces')} isLabelHidden columns={columns}>
          {#each workspaces as workspace (workspace.id)}
              <tr>
                <td>
                  <span class="name">{workspace.display_name}</span>
                  <span class="slug">{workspace.slug}</span>
                </td>
                <td>
                  <Badge
                    tone={workspace.status === 'ACTIVE'
                      ? 'success'
                      : workspace.status === 'SUSPENDED'
                        ? 'warning'
                        : 'danger'}
                  >
                    {t(`app.instance.state_${workspace.status.toLowerCase()}`)}
                  </Badge>
                  {#if workspace.purge_after}
                    <!-- The grace, where one stands: the number an operator needs before deciding
                         whether there is still time to resume. -->
                    <span class="slug">{t('app.instance.purge_after', { at: workspace.purge_after })}</span>
                  {/if}
                </td>
                <td class="slug">{workspace.created_at}</td>
                <td>
                  {#if workspace.status === 'ACTIVE'}
                    <Button
                      tone="subtle"
                      isBusy={working === workspace.id}
                      busyLabel={t('app.instance.working')}
                      onclick={() => void shift(workspace.id, 'suspend')}
                    >
                      {t('app.instance.suspend')}
                    </Button>
                  {:else if workspace.status === 'SUSPENDED'}
                    <Button
                      tone="subtle"
                      isBusy={working === workspace.id}
                      busyLabel={t('app.instance.working')}
                      onclick={() => void shift(workspace.id, 'resume')}
                    >
                      {t('app.instance.resume')}
                    </Button>
                  {/if}
                </td>
              </tr>
          {/each}
        </Table>
      {/if}
    </Stack>
  </InstanceGate>
</Stack>

<style>
  .quiet { margin: 0; display: flex; align-items: center; gap: var(--sp-100); color: var(--text-secondary); }
  .name { display: block; font-weight: var(--fw-medium); }
  .slug { display: block; color: var(--text-secondary); font-size: var(--fs-100); }
</style>
