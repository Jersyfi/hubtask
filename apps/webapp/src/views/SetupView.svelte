<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The first start (UC-INS-01): an installation with no workspace and nobody to run it, opened in
  // a browser. One card creates the first workspace, its owner - the person in front of it - and
  // the owner's place among the installation's operators, and signs them in.
  //
  // **Drawn only while the manifest says so** (`features.setup_open`), and the sign-in card
  // otherwise: once anything exists the server refuses every setup, and a card that offered one
  // would be a control the server refuses (P-05).
  //
  // **The code is the only key.** The server printed it on its console at start; this card says
  // where to find it and that it lasts thirty minutes, and the field takes it however it is pasted.
  //
  // **No "operator", no "installation lock", no short name in single mode** (P-10): a person setting
  // up Hubtask for themselves names their workspace and themselves, and that is all. Multi mode asks
  // for the short name, because it becomes the workspace's address.
  //
  // **The server's own rules are judged when the form is sent.** There is no account yet whose
  // proof a check could present, so the password's lines show what can be seen locally and say
  // the rest is checked on sending.

  import { Banner, Button, Input, Stack } from '@hubtask/design-system/components';

  import PasswordField from '../lib/signin/PasswordField.svelte';
  import SignInCard from '../lib/signin/SignInCard.svelte';
  import { manifest } from '../lib/data/capabilities.svelte.ts';
  import { signInRules } from '../lib/data/signinrules.svelte.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { session } from '../lib/session.svelte.ts';

  interface Props {
    /** Where to go once the owner is signed in. */
    onnavigate?: (path: string) => void;
  }

  let { onnavigate }: Props = $props();

  let code = $state('');
  let workspace = $state('');
  let slug = $state('');
  let name = $state('');
  let email = $state('');
  let password = $state('');

  const isBusy = $derived(session.status === 'verifying');
  const asksShortName = $derived(manifest.isMultiTenant);
  const fieldError = (path: string): string | undefined => session.problem?.fields.get(path);
  /** A problem that belongs to a field is said there and not twice. */
  const isFieldProblem = $derived(
    session.problem !== undefined &&
      ['/code', '/slug', '/display_name', '/owner_email', '/password'].some((path) => session.problem?.fields.has(path)),
  );

  $effect(() => signInRules.read());

  async function submit(event: SubmitEvent): Promise<void> {
    event.preventDefault();
    const signedIn = await session.setUp({
      code,
      display_name: workspace,
      ...(asksShortName ? { slug } : {}),
      owner_email: email,
      ...(name.trim() ? { owner_display_name: name } : {}),
      password,
      default_locale: messages.locale,
    });
    if (signedIn) {
      password = '';
      onnavigate?.('/');
    }
  }
</script>

{#snippet notice()}
  {#if session.problem && !isFieldProblem}
    <Banner tone="danger" title={session.problem.message}>
      {#if session.problem.reference}{session.problem.reference}{/if}
    </Banner>
  {/if}
{/snippet}

<SignInCard title={t('app.setup.title')} lead={t('app.setup.lead')} {notice}>
  <form onsubmit={submit}>
    <Stack gap="200">
      <Input
        label={t('app.setup.code_label')}
        hint={t('app.setup.code_hint')}
        error={fieldError('/code')}
        bind:value={code}
        autocomplete="one-time-code"
        autocapitalize="characters"
        spellcheck={false}
        isRequired
      />
      <Input
        label={t('app.setup.workspace_label')}
        hint={t('app.setup.workspace_hint')}
        error={fieldError('/display_name')}
        bind:value={workspace}
        autocomplete="organization"
        isRequired
      />
      {#if asksShortName}
        <Input
          label={t('app.setup.slug_label')}
          hint={t('app.setup.slug_hint', { address: `${slug || '…'}.${location.host}` })}
          error={fieldError('/slug')}
          bind:value={slug}
          autocapitalize="none"
          spellcheck={false}
          isRequired
        />
      {/if}
      <Input
        label={t('app.setup.name_label')}
        bind:value={name}
        autocomplete="name"
      />
      <Input
        label={t('app.sign_in.email_label')}
        hint={t('app.setup.email_hint')}
        error={fieldError('/owner_email')}
        bind:value={email}
        type="email"
        autocomplete="username"
        spellcheck={false}
        isRequired
      />
      <PasswordField
        bind:value={password}
        label={t('app.setup.password_label')}
        context={{ email, displayName: name, workspaceName: workspace }}
        proof={{ kind: 'none' }}
        error={fieldError('/password')}
        hint={t('app.redeem.password_hint')}
      />
      <Button type="submit" tone="primary" isFull {isBusy} busyLabel={t('app.setup.working')}>
        {t('app.setup.submit')}
      </Button>
    </Stack>
  </form>
</SignInCard>

<style>
  form { margin: 0; }
</style>
