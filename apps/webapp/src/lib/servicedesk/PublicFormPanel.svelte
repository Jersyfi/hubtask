<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A collection's public form, for who manages its members (UC-SVC-13 check 1): whether it has
  // one and who made it, a link made or renewed - its address shown once, since only a hash of
  // the token is kept - and taken away. Requests sent through it are filed as the person who made
  // it, which the sentence says. Nothing is drawn for somebody the server does not let read it.

  import { untrack } from 'svelte';
  import { Button, Inline, Input, Stack } from '@hubtask/design-system/components';
  import type { PublicForm, PublicLinksPolicy } from '@hubtask/sync-engine';

  import { publicFormAddress } from '../data/publicforms.ts';
  import { publicFormPath, publicForms, publicFormsPolicyPath } from '../data/publicforms.svelte.ts';
  import { resource, resourceWhen } from '../data/resource.svelte.ts';
  import { toast } from '../frame/toasts.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';

  // `hasForm` is the catalogue's word on whether there is one; false reads nothing, since the
  // link's read would answer 404 and the browser log it.
  const { collectionId, hasForm }: { collectionId: string; hasForm?: boolean } = $props();

  const link = resourceWhen<PublicForm>(() => (hasForm === false ? undefined : publicFormPath(collectionId)));
  const policy = untrack(() => resource<PublicLinksPolicy>(publicFormsPolicyPath));
  const allowed = $derived(policy.state.status === 'ready' && policy.state.data.allowed);
  const held = $derived(link.state.status === 'ready' ? link.state.data : undefined);
  // A refusal other than "it has none" means the reader may not manage it: the panel stays away.
  const absent = $derived(hasForm === false || (link.state.status === 'failed' && link.state.error.detailCode === 'public_forms.link_not_found'));

  let madeAddress = $state<string | undefined>(undefined);
  let refusal = $state<string | undefined>(undefined);

  async function make() {
    refusal = undefined;
    try {
      const made = await publicForms.make(collectionId);
      madeAddress = publicFormAddress(globalThis.location.origin, made.token);
      void navigator.clipboard?.writeText(madeAddress).catch(() => undefined);
      toast.show({ text: t('app.public_form.made'), tone: 'success' });
    } catch (error) {
      refusal = renderProblem(error as never, messages).message;
    }
  }

  async function remove() {
    refusal = undefined;
    try {
      await publicForms.remove(collectionId);
      madeAddress = undefined;
      toast.show({ text: t('app.public_form.removed'), tone: 'success' });
    } catch (error) {
      refusal = renderProblem(error as never, messages).message;
    }
  }
</script>

{#if held || (absent && allowed)}
  <section class="panel" data-public-form aria-labelledby="public-form-heading">
    <Stack gap="100">
      <h2 class="heading" id="public-form-heading">{t('app.public_form.title')}</h2>
      {#if held}
        <p class="quiet">{allowed ? t('app.public_form.open') : t('app.public_form.paused')}</p>
      {:else}
        <p class="quiet">{t('app.public_form.none')}</p>
      {/if}
      {#if madeAddress}
        <Input label={t('app.public_form.address')} value={madeAddress} readonly />
        <p class="quiet">{t('app.public_form.once')}</p>
      {/if}
      <Inline gap="100" isWrapping>
        {#if allowed}
          <Button size="sm" tone="secondary" icon="link" onclick={make}>
            {held ? t('app.public_form.renew') : t('app.public_form.make')}
          </Button>
        {/if}
        {#if held}
          <Button size="sm" tone="subtle" onclick={remove}>{t('app.public_form.remove')}</Button>
        {/if}
      </Inline>
      <p class="quiet">{t('app.public_form.hint')}</p>
      {#if refusal}<p class="refusal" role="alert">{refusal}</p>{/if}
    </Stack>
  </section>
{/if}

<style>
  .panel { max-inline-size: 72ch; }
  .heading { margin: 0; font-size: var(--fs-100); font-weight: var(--fw-semibold); }
  .quiet { margin: 0; color: var(--text-secondary); }
  .refusal { margin: 0; color: var(--text-danger); }
</style>
