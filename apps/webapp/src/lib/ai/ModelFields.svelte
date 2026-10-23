<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The one model form, for a workspace's own model and for a model the installation offers every
  // workspace: which kind, where it is reached, its two models, the key and where it processes
  // content. One form, so a field reads and saves alike at both levels. What only one level asks
  // comes in as `before` (the installation's name and processor) and `more` (the workspace's
  // minimum and consent).
  //
  // **The key goes one way.** Sealed on the way in, and `has_api_key` its only echo, so a stored
  // model shows an empty field that says leaving it empty keeps the sealed key.
  //
  // **The kinds and declarations are the contract's own enums**, offered as the level permits: a
  // model that calls nothing is no model to offer, and a declaration below the minimum is refused
  // by the server, so neither is drawn (P-05).

  import type { Snippet } from 'svelte';

  import { Input, Select, Stack } from '@hubtask/design-system/components';
  import type { AiProviderKind } from '@hubtask/sync-engine';

  import { admittedJurisdictions, type ModelDraft } from './model.ts';
  import type { Missing } from '../forms/missing.ts';
  import { t } from '../i18n/i18n.svelte.ts';

  interface Props {
    draft: ModelDraft;
    /** The kinds this level offers. */
    kinds: readonly AiProviderKind[];
    /** The installation's minimum declaration, `ANY` where there is none. */
    minimum?: string;
    /** The declaration stored, kept in the list even below the minimum. */
    stored?: string;
    /** Whether a key is sealed already. */
    hasKey: boolean;
    /** Whether the address must be given rather than left to the provider's default. */
    isAddressRequired?: boolean;
    /** What a press of save found missing, and the server's refusals by field. */
    missing?: Missing;
    errors?: ReadonlyMap<string, string>;
    before?: Snippet;
    more?: Snippet;
  }

  let {
    draft = $bindable(),
    kinds,
    minimum = 'ANY',
    stored,
    hasKey,
    isAddressRequired = false,
    missing = {},
    errors,
    before,
    more,
  }: Props = $props();

  const kindOptions = $derived(kinds.map((value) => ({ value, label: t(`app.ai.kind_${value.toLowerCase()}`) })));
  const jurisdictionOptions = $derived(
    admittedJurisdictions(minimum, stored as ModelDraft['jurisdiction']).map((value) => ({
      value,
      label: t(`app.ai.jurisdiction_${value.toLowerCase()}`),
    })),
  );
</script>

<Stack gap="200">
  {@render before?.()}
  <Select
    label={t('app.ai.kind_label')}
    hint={t('app.ai.kind_hint')}
    bind:value={draft.kind}
    placeholder={t('app.ai.kind_choose')}
    options={kindOptions}
    error={missing.kind && !draft.kind ? t(missing.kind) : undefined}
  />
  <Input
    label={t('app.ai.base_url_label')}
    hint={isAddressRequired ? t('app.ai.base_url_hint_required') : t('app.ai.base_url_hint')}
    bind:value={draft.baseUrl}
    error={errors?.get('/base_url')}
    type="url"
    autocomplete="off"
    spellcheck={false}
    isRequired={isAddressRequired}
  />
  <Input label={t('app.ai.completion_model_label')} hint={t('app.ai.completion_model_hint')} bind:value={draft.completionModel} autocomplete="off" spellcheck={false} />
  <Input label={t('app.ai.embedding_model_label')} hint={t('app.ai.embedding_model_hint')} bind:value={draft.embeddingModel} autocomplete="off" spellcheck={false} />
  <Input
    label={t('app.ai.api_key_label')}
    hint={hasKey ? t('app.ai.api_key_kept') : t('app.ai.api_key_hint')}
    bind:value={draft.apiKey}
    type="password"
    autocomplete="off"
    spellcheck={false}
  />
  <Select
    label={t('app.ai.jurisdiction_label')}
    hint={t('app.ai.jurisdiction_hint')}
    bind:value={draft.jurisdiction}
    placeholder={t('app.ai.jurisdiction_choose')}
    options={jurisdictionOptions}
    error={missing.jurisdiction && !draft.jurisdiction ? t(missing.jurisdiction) : undefined}
  />
  {@render more?.()}
</Stack>
