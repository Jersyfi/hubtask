<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // *Publish* (scene postwrite 1 and 3, B-3, B-5, B-6): now or at a day and time; who sees it,
  // which only narrows; notify; a read confirmation with its deadline; featured until; the
  // discussion. On a published post the same panel changes what still can change. It stands in the
  // popover its button opens.

  import { Button, Checkbox, CheckboxGroup, Input, Select } from '@hubtask/design-system/components';
  import type { Post, PostSettings } from '@hubtask/sync-engine';
  import { untrack } from 'svelte';

  import { groups } from '../data/groups.svelte.ts';
  import { posts, settingsOf } from '../data/posts.svelte.ts';
  import { toast } from '../frame/toasts.svelte.ts';
  import { formatDateTime } from '../i18n/datetime.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';
  import { atLocal, dayOf, timeOf } from './moments.ts';

  interface Props {
    post: Post;
    hubName: string;
    /** Every member of the workspace reads the hub: its readers are *everyone in the workspace*. */
    isWorkspaceWide: boolean;
    onclose: () => void;
    ondone: (post: Post) => void;
  }

  const { post, hubName, isWorkspaceWide, onclose, ondone }: Props = $props();

  // The panel opens on what the post stands with; what the writer changes stays the panel's own.
  const was = untrack(() => post);
  const isPublished = was.state === 'PUBLISHED';
  const week = (days: number) => new Date(Date.now() + days * 86_400_000).toISOString();
  let isScheduled = $state(was.state === 'SCHEDULED');
  let day = $state(dayOf(was.publish_at ?? week(1)));
  let time = $state(timeOf(was.publish_at ?? '') || '08:00');
  let audience = $state<PostSettings['audience']>(was.audience);
  let chosenGroups = $state<readonly string[]>(was.group_ids);
  let notify = $state(was.notify);
  let asksConfirmation = $state(!!was.confirm_by);
  let confirmDay = $state(dayOf(was.confirm_by ?? week(7)));
  let isFeatured = $state(!!was.featured_until);
  let featureDay = $state(dayOf(was.featured_until ?? week(14)));
  let discussion = $state(was.discussion);
  let isBusy = $state(false);

  $effect(() => groups.open());

  const scheduledAt = $derived(atLocal(day, time));
  const primary = $derived(
    isPublished ? t('app.posts.save') : isScheduled && scheduledAt ? t('app.posts.schedule_for', { when: formatDateTime(scheduledAt, messages.locale) }) : t('app.posts.publish_now'),
  );

  async function save() {
    if (isBusy) return;
    const settings: PostSettings = {
      ...settingsOf(post),
      publish_at: isPublished ? post.publish_at : isScheduled ? (scheduledAt ?? null) : null,
      audience,
      group_ids: audience === 'GROUPS' ? [...chosenGroups] : [],
      notify,
      discussion,
      confirm_by: asksConfirmation ? (atLocal(confirmDay, '23:59') ?? null) : null,
      featured_until: isFeatured ? (atLocal(featureDay, '23:59') ?? null) : null,
    };
    isBusy = true;
    try {
      let next = await posts.settle(post, settings);
      if (!isPublished && !isScheduled) next = await posts.publish(next);
      toast.show({ text: t(isPublished ? 'app.posts.saved' : isScheduled ? 'app.posts.scheduled' : 'app.posts.published'), tone: 'success' });
      ondone(next);
    } catch (error) {
      toast.show({ text: renderProblem(error as never, messages).message, tone: 'danger' });
    } finally {
      isBusy = false;
    }
  }
</script>

<section class="publish" aria-labelledby="post-publish" data-post-publish>
  <h2 id="post-publish">{t(isPublished ? 'app.posts.settings' : 'app.posts.publish')}</h2>
  {#if !isPublished}
    <div class="when" role="radiogroup" aria-label={t('app.posts.when')}>
      <button type="button" role="radio" aria-checked={!isScheduled} data-post-when="now" onclick={() => (isScheduled = false)}>{t('app.posts.now')}</button>
      <button type="button" role="radio" aria-checked={isScheduled} data-post-when="schedule" onclick={() => (isScheduled = true)}>{t('app.posts.schedule')}</button>
    </div>
    {#if isScheduled}
      <div class="row">
        <Input type="date" size="sm" label={t('app.posts.day')} bind:value={day} data-post-day />
        <Input type="time" size="sm" label={t('app.posts.time')} bind:value={time} data-post-time />
      </div>
    {/if}
  {/if}
  <Select size="sm" label={t('app.posts.audience')} hint={t('app.posts.audience_hint')} bind:value={audience} data-post-audience
    options={[
      { value: 'HUB', label: isWorkspaceWide ? t('app.posts.audience_workspace') : t('app.posts.audience_hub', { hub: hubName }) },
      { value: 'GROUPS', label: t('app.posts.audience_groups') },
    ]} />
  {#if audience === 'GROUPS'}
    <CheckboxGroup label={t('app.posts.groups')} selected={chosenGroups} onChange={(next) => (chosenGroups = next)}
      options={groups.all.map((group) => ({ value: group.id, label: group.name }))} />
  {/if}
  <Checkbox label={t('app.posts.notify')} hint={t('app.posts.notify_hint')} bind:checked={notify} data-post-notify />
  <Checkbox label={t('app.posts.confirm')} hint={t('app.posts.confirm_hint')} bind:checked={asksConfirmation} data-post-confirm />
  {#if asksConfirmation}<Input type="date" size="sm" label={t('app.posts.confirm_by')} bind:value={confirmDay} data-post-confirm-by />{/if}
  <Checkbox label={t('app.posts.feature')} hint={t('app.posts.feature_hint')} bind:checked={isFeatured} data-post-feature />
  {#if isFeatured}<Input type="date" size="sm" label={t('app.posts.feature_until')} bind:value={featureDay} data-post-feature-until />{/if}
  <Checkbox label={t('app.posts.discussion_on')} hint={t('app.posts.discussion_hint')} bind:checked={discussion} data-post-discussion-on />
  <div class="foot">
    <Button size="sm" tone="subtle" onclick={onclose}>{t('app.posts.cancel')}</Button>
    <Button size="sm" tone="primary" {isBusy} onclick={() => void save()} data-post-publish-go
      disabledReason={!isPublished && isScheduled && !scheduledAt ? t('app.posts.when_missing') : audience === 'GROUPS' && chosenGroups.length === 0 ? t('app.posts.groups_missing') : undefined}>{primary}</Button>
  </div>
</section>

<style>
  .publish { display: flex; flex-direction: column; gap: var(--sp-150); }
  h2 { margin: 0; font-size: var(--fs-200); font-weight: var(--fw-semibold); }
  .when { display: grid; grid-template-columns: 1fr 1fr; border: var(--bw-hairline) solid var(--border-subtle); border-radius: var(--r-md); overflow: hidden; }
  .when button { border: none; background: none; padding: var(--sp-100); font: inherit; cursor: pointer; color: var(--text-secondary); }
  .when button[aria-checked='true'] { background: var(--accent-primary-subtle); color: var(--text-primary); font-weight: var(--fw-semibold); }
  .when button:focus-visible { outline: var(--bw-ring) solid var(--focus-ring); outline-offset: calc(-1 * var(--bw-ring)); }
  .row { display: grid; grid-template-columns: minmax(0, 2fr) minmax(0, 1fr); gap: var(--sp-100); }
  .foot { display: flex; justify-content: flex-end; gap: var(--sp-100); }

  /* A thumb's target (§7): where the pointer is coarse or the window a phone's, the control's press
     reaches layout.target, by its own height or beyond its drawn box. */
  /* design-system-lint-ignore: `primitive.breakpoint.medium` (600px); a media query cannot read a custom property. */
  @media (pointer: coarse), (width < 600px) {
    .when button { min-block-size: var(--layout-target); }
  }
</style>
