<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // One configuration item at `/cis/:id` (UC-AST-01 check 8): a detail page of its own, apart from
  // the hub's assets, which are a work page (page-system §7.8, PS-1). Whether the reader may write
  // is read from their role in the item's hub (P-05).
  import { untrack } from 'svelte';
  import { DetailPage, Spinner } from '@hubtask/design-system/components';
  import type { CiClassList, CiRelationTypeList, ConfigurationItem } from '@hubtask/sync-engine';

  import CiDetail from '../lib/assets/CiDetail.svelte';
  import { actor } from '../lib/data/account.svelte.ts';
  import { holds, roleOrder } from '../lib/data/capability.svelte.ts';
  import { ciPath, classesPath, relationTypesPath, type Words } from '../lib/data/cmdb.svelte.ts';
  import { containers } from '../lib/data/containers.svelte.ts';
  import { people } from '../lib/data/people.svelte.ts';
  import { resource } from '../lib/data/resource.svelte.ts';
  import FailureNote from '../lib/frame/FailureNote.svelte';
  import { page as frame } from '../lib/frame/page.svelte.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { renderProblem } from '../lib/problem.ts';

  interface Props {
    ciId: string;
    onnavigate: (path: string) => void;
  }

  const { ciId, onnavigate }: Props = $props();

  const opened = untrack(() => resource<ConfigurationItem>(ciPath(ciId)));
  const current = $derived(opened.state.status === 'ready' ? opened.state.data : undefined);
  const openRefusal = $derived(opened.state.status === 'failed' ? renderProblem(opened.state.error, messages).message : undefined);
  const hubId = $derived(current?.hub_id ?? '');
  const hub = $derived(hubId ? containers.find(hubId) : undefined);

  const classesRead = resource<CiClassList>(classesPath);
  const typesRead = resource<CiRelationTypeList>(relationTypesPath);
  const classes = $derived(classesRead.state.status === 'ready' ? classesRead.state.data.data : []);
  const types = $derived(typesRead.state.status === 'ready' ? typesRead.state.data.data : []);

  const words: Words = (code) => (messages.has(code) ? t(code) : undefined);

  $effect(() => {
    const at = hubId;
    if (!at) return;
    return untrack(() => people.open({ hubId: at }));
  });
  const mayWrite = $derived(
    hubId ? holds(people.roleOf({ hubId }, actor.account?.id, roleOrder()) ?? undefined, 'WRITE_ITEMS').status === 'permitted' : false,
  );

  $effect(() => frame.entitle(current?.name ?? t('app.assets.title')));
</script>

{#if current && classesRead.state.status === 'ready'}
  {#key current.id}
    <CiDetail ci={current} {classes} {types} {words} {mayWrite} {onnavigate} hub={hub ? { id: hub.id, name: hub.name } : undefined} />
  {/key}
{:else}
  <DetailPage head={{ title: t('app.assets.title') }}>
    {#if openRefusal}
      <FailureNote problem={openRefusal} />
    {:else}
      <Spinner label={t('app.assets.loading')} />
    {/if}
  </DetailPage>
{/if}
