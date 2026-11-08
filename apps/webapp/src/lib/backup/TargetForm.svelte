<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A backup target's creation form - the workspace's and the installation's share it, because a
  // target is one thing wherever it belongs, and two forms would be two sets of field rules.
  //
  // It stands in *Add a target*, a Dialog M: the dialog carries the heading and the primary, and
  // what most targets leave as it is - the encryption, a bucket's region, an SFTP key - is folded
  // under *Advanced* (page-system §5.1).
  //
  // **The credentials are write-only by construction**: typed here, handed to the owner of the form
  // once, and never read back into it.

  import { tick } from 'svelte';

  import { Banner, Checkbox, Disclosure, Input, Select, Stack, Textarea } from '@hubtask/design-system/components';

  import type { Target, TargetDraft } from '../data/backup.svelte.ts';
  import { insecureReasons, sftpTarget, webdavTarget } from '../data/backup.ts';
  import { check, focusFirst, isComplete, type Missing } from '../forms/missing.ts';
  import { t } from '../i18n/i18n.svelte.ts';

  interface Props {
    /** Adds the target; true when it was added, so the form empties. */
    onadd: (draft: TargetDraft) => Promise<boolean>;
    /** The form, for a dialog's primary to submit. */
    form?: HTMLFormElement;
    /** Whether anything is typed or chosen: what a dialog's *Discard changes?* asks about. */
    isTouched?: boolean;
  }

  let { onadd, form = $bindable(), isTouched = $bindable(false) }: Props = $props();

  /**
   * The kinds this build speaks (UC-BAK-01 check 10). The other names in the contract are refused
   * by name until an adapter exists for them (check 2), so offering them would offer a refusal.
   */
  const KINDS = ['LOCAL', 'S3', 'SFTP', 'WEBDAV'] as const;

  // The target form. `credentials` is write-only by construction: it is never read back into it.
  let draftName = $state('');
  let draftKind = $state<string>('LOCAL');
  let draftPath = $state('');
  let draftEndpoint = $state('');
  let draftBucket = $state('');
  let draftAccessKey = $state('');
  let draftSecretKey = $state('');
  let draftHost = $state('');
  let draftUser = $state('');
  let draftPassword = $state('');
  let draftHostKey = $state('');
  let draftRegion = $state('');
  let draftUrl = $state('');
  let draftPrivateKey = $state('');
  let draftKeyPassphrase = $state('');
  /** Encrypted unless the owner says otherwise, which is the contract's default too. */
  let draftEncryption = $state('AES256_GCM');
  let draftAccepted = $state(false);

  /** What the chosen kind needs, as the contract's own `config`. Nothing here is a credential. */
  function configuration(): Record<string, unknown> {
    if (draftKind === 'S3') {
      return { endpoint: draftEndpoint.trim(), bucket: draftBucket.trim(), ...(draftRegion.trim() ? { region: draftRegion.trim() } : {}) };
    }
    if (draftKind === 'SFTP') return sftpTarget(sftpDraft()).config;
    if (draftKind === 'WEBDAV') return webdavTarget(webdavDraft()).config;
    return { path: draftPath.trim() };
  }

  const sftpDraft = () => ({
    host: draftHost, path: draftPath, username: draftUser, hostKey: draftHostKey, password: draftPassword,
    privateKey: draftPrivateKey, keyPassphrase: draftKeyPassphrase,
  });
  const webdavDraft = () => ({ url: draftUrl, username: draftUser, password: draftPassword });

  /**
   * Why the server will ask the owner to accept this target, as its own sentences - asked here,
   * before the add, rather than met as a refusal after it (P-05).
   */
  const insecure = $derived(insecureReasons(draftKind, configuration(), draftEncryption));

  /** What goes in once and is never answered. */
  function credentials(): Record<string, unknown> | undefined {
    if (draftKind === 'S3' && draftAccessKey) {
      return { access_key: draftAccessKey, secret_key: draftSecretKey };
    }
    if (draftKind === 'SFTP') return sftpTarget(sftpDraft()).credentials;
    if (draftKind === 'WEBDAV') return webdavTarget(webdavDraft()).credentials;
    return undefined;
  }

  /** What a press of *Add* found missing - a name, the acceptance of a risk - marked at the fields. */
  let missing = $state<Missing>({});

  $effect(() => {
    isTouched = [draftName, draftPath, draftEndpoint, draftBucket, draftAccessKey, draftSecretKey, draftHost, draftUser, draftPassword, draftHostKey, draftRegion, draftUrl, draftPrivateKey]
      .some((typed) => typed.trim() !== '') || draftKind !== 'LOCAL' || draftEncryption !== 'AES256_GCM';
  });

  async function createTarget(event: SubmitEvent): Promise<void> {
    event.preventDefault();
    // Never switched off for what is missing: pressed, it is marked and focused (UC-WRK-32 check 3).
    missing = check([
      { kind: 'field', key: 'name', code: 'app.backup.name_needed', isFilled: draftName.trim() !== '' },
      { kind: 'field', key: 'accepted', code: 'app.backup.insecure_first', isFilled: insecure.length === 0 || draftAccepted },
    ]);
    if (!isComplete(missing)) {
      void tick().then(() => focusFirst(form));
      return;
    }
    const added = await onadd({
      name: draftName.trim(),
      kind: draftKind as Target['kind'],
      config: configuration(),
      ...(credentials() ? { credentials: credentials() } : {}),
      encryption_mode: draftEncryption,
      // Only when there is something to accept, and only once the owner has accepted it.
      ...(insecure.length > 0 && draftAccepted ? { insecure_acknowledged: true } : {}),
    });
    if (!added) return;
    draftName = '';
    draftPath = '';
    draftEndpoint = '';
    draftBucket = '';
    draftAccessKey = '';
    draftSecretKey = '';
    draftHost = '';
    draftUser = '';
    draftPassword = '';
    draftHostKey = '';
    draftRegion = '';
    draftUrl = '';
    draftPrivateKey = '';
    draftKeyPassphrase = '';
    draftEncryption = 'AES256_GCM';
    draftAccepted = false;
  }
</script>

{#snippet encryption()}
  <Select
    label={t('app.backup.encryption')}
    hint={t('app.backup.encryption_hint')}
    bind:value={draftEncryption}
    options={[
      { value: 'AES256_GCM', label: t('app.backup.encryption_aes') },
      { value: 'NONE', label: t('app.backup.encryption_none') },
    ]}
  />
{/snippet}

{#snippet region()}
  <Input label={t('app.backup.target_region')} hint={t('app.backup.target_region_hint')} bind:value={draftRegion} />
{/snippet}

{#snippet privateKey()}
  <!-- A key is a credential like the password: typed in, sent once, never shown back. -->
  <Textarea
    label={t('app.backup.target_private_key')}
    hint={t('app.backup.target_private_key_hint')}
    rows={4}
    spellcheck={false}
    autocomplete="off"
    bind:value={draftPrivateKey}
  />
  {#if draftPrivateKey.trim()}
    <Input label={t('app.backup.target_key_passphrase')} type="password" bind:value={draftKeyPassphrase} />
  {/if}
{/snippet}

<form bind:this={form} onsubmit={createTarget}>
  <Stack gap="150">
    <Input label={t('app.backup.target_name')} bind:value={draftName} isRequired error={missing.name && !draftName.trim() ? t(missing.name) : undefined} />
    <Select
      label={t('app.backup.target_kind')}
      bind:value={draftKind}
      options={KINDS.map((kind) => ({ value: kind, label: t(`app.backup.kind_${kind}`) }))}
    />
    {#if draftKind === 'LOCAL'}
      <Input
        label={t('app.backup.target_path')}
        hint={t('app.backup.target_path_hint')}
        bind:value={draftPath}
      />
    {:else if draftKind === 'S3'}
      <div class="group pair">
        <Input label={t('app.backup.target_endpoint')} bind:value={draftEndpoint} />
        <Input label={t('app.backup.target_bucket')} bind:value={draftBucket} />
      </div>
      <div class="group pair">
        <Input label={t('app.backup.target_access_key')} bind:value={draftAccessKey} />
        <Input
          label={t('app.backup.target_secret_key')}
          type="password"
          bind:value={draftSecretKey}
        />
      </div>
    {:else if draftKind === 'WEBDAV'}
      <Input
        label={t('app.backup.target_url')}
        hint={t('app.backup.target_url_hint')}
        type="url"
        bind:value={draftUrl}
        isRequired
      />
      <div class="group pair">
        <Input label={t('app.backup.target_user')} bind:value={draftUser} />
        <Input label={t('app.backup.target_password')} type="password" bind:value={draftPassword} />
      </div>
    {:else}
      <div class="group pair">
        <Input label={t('app.backup.target_host')} bind:value={draftHost} />
        <Input label={t('app.backup.target_path')} bind:value={draftPath} />
      </div>
      <Textarea
        label={t('app.backup.target_host_key')}
        hint={t('app.backup.target_host_key_hint')}
        rows={3}
        spellcheck={false}
        bind:value={draftHostKey}
        isRequired
      />
      <div class="group pair">
        <Input label={t('app.backup.target_user')} bind:value={draftUser} />
        <Input
          label={t('app.backup.target_password')}
          type="password"
          bind:value={draftPassword}
        />
      </div>
    {/if}
    <!-- The risk an unencrypted target carries is said below the fold, never inside it. -->
    <Disclosure tone="quiet" label={t('app.backup.target_advanced')} description={t('app.backup.target_advanced_says')}>
      <Stack gap="150">
        {#if draftKind === 'S3'}{@render region()}{/if}
        {#if draftKind === 'SFTP'}{@render privateKey()}{/if}
        {@render encryption()}
      </Stack>
    </Disclosure>
    {#if insecure.length > 0}
      <!-- The risk in the server's own words, and the owner's acceptance of it, before the
           add: the server refuses such a target without it (UC-BAK-01 check 6). -->
      <Banner tone="warning" title={t('app.backup.insecure_title')}>
        <ul class="reasons">
          {#each insecure as reason (reason)}<li>{t(reason)}</li>{/each}
        </ul>
      </Banner>
      <Checkbox label={t('app.backup.insecure_accept')} bind:checked={draftAccepted} error={missing.accepted && !draftAccepted ? t(missing.accepted) : undefined} />
    {/if}
    <!-- Said where the credentials are typed: they go in and the contract answers `config`
         without them, so there is nothing to come back and check later. -->
    <p class="quiet small">{t('app.backup.credentials_write_only')}</p>
  </Stack>
</form>

<style>
  .quiet { margin: 0; color: var(--text-secondary); }

  .small { font-size: var(--fs-075); }

  form { margin: 0; }

  .reasons { margin: 0; padding-inline-start: var(--sp-200); }

  /* Without a pair, the two fields stack as every other field of the form does. */
  /* Two short fields that belong together share a line in a dialog, so the form needs no scrolling
     (page-system §5.1); a full-screen dialog on a phone stacks them again. */
  .pair { display: grid; grid-template-columns: 1fr 1fr; gap: var(--sp-150); }

  /* design-system-lint-ignore: `primitive.breakpoint.medium` (600px); a media query cannot read a custom property. */
  @media (width < 600px) {
    .pair { grid-template-columns: 1fr; }
  }
</style>
