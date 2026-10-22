<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A backup target's creation form - the workspace's and the installation's share it, because a
  // target is one thing wherever it belongs, and two forms would be two sets of field rules.
  //
  // **The credentials are write-only by construction**: typed here, handed to the owner of the form
  // once, and never read back into it.

  import { Banner, Button, Checkbox, Input, Select, Stack, Textarea } from '@hubtask/design-system/components';

  import type { Target, TargetDraft } from '../data/backup.svelte.ts';
  import { insecureReasons, sftpTarget, webdavTarget } from '../data/backup.ts';
  import { t } from '../i18n/i18n.svelte.ts';

  interface Props {
    heading: string;
    isWorking: boolean;
    /** Adds the target; true when it was added, so the form empties. */
    onadd: (draft: TargetDraft) => Promise<boolean>;
  }

  const { heading, isWorking, onadd }: Props = $props();

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

  async function createTarget(event: SubmitEvent): Promise<void> {
    event.preventDefault();
    if (!draftName.trim()) return;
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

    <form class="panel" onsubmit={createTarget}>
      <Stack gap="150">
        <h3 class="section">{heading}</h3>
        <Input label={t('app.backup.target_name')} bind:value={draftName} isRequired />
        <Select
          label={t('app.backup.target_kind')}
          bind:value={draftKind}
          options={KINDS.map((kind) => ({ value: kind, label: kind }))}
        />
        {#if draftKind === 'LOCAL'}
          <Input
            label={t('app.backup.target_path')}
            hint={t('app.backup.target_path_hint')}
            bind:value={draftPath}
          />
        {:else if draftKind === 'S3'}
          <Input label={t('app.backup.target_endpoint')} bind:value={draftEndpoint} />
          <Input label={t('app.backup.target_bucket')} bind:value={draftBucket} />
          <Input label={t('app.backup.target_region')} hint={t('app.backup.target_region_hint')} bind:value={draftRegion} />
          <Input label={t('app.backup.target_access_key')} bind:value={draftAccessKey} />
          <Input
            label={t('app.backup.target_secret_key')}
            type="password"
            bind:value={draftSecretKey}
          />
        {:else if draftKind === 'WEBDAV'}
          <Input
            label={t('app.backup.target_url')}
            hint={t('app.backup.target_url_hint')}
            type="url"
            bind:value={draftUrl}
            isRequired
          />
          <Input label={t('app.backup.target_user')} bind:value={draftUser} />
          <Input label={t('app.backup.target_password')} type="password" bind:value={draftPassword} />
        {:else}
          <Input label={t('app.backup.target_host')} bind:value={draftHost} />
          <Input label={t('app.backup.target_path')} bind:value={draftPath} />
          <Input label={t('app.backup.target_user')} bind:value={draftUser} />
          <Textarea
            label={t('app.backup.target_host_key')}
            hint={t('app.backup.target_host_key_hint')}
            rows={3}
            spellcheck={false}
            bind:value={draftHostKey}
            isRequired
          />
          <Input
            label={t('app.backup.target_password')}
            type="password"
            bind:value={draftPassword}
          />
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
        {/if}
        <Select
          label={t('app.backup.encryption')}
          hint={t('app.backup.encryption_hint')}
          bind:value={draftEncryption}
          options={[
            { value: 'AES256_GCM', label: t('app.backup.encryption_aes') },
            { value: 'NONE', label: t('app.backup.encryption_none') },
          ]}
        />
        {#if insecure.length > 0}
          <!-- The risk in the server's own words, and the owner's acceptance of it, before the
               add: the server refuses such a target without it (UC-BAK-01 check 6). -->
          <Banner tone="warning" title={t('app.backup.insecure_title')}>
            <ul class="reasons">
              {#each insecure as reason (reason)}<li>{t(reason)}</li>{/each}
            </ul>
          </Banner>
          <Checkbox label={t('app.backup.insecure_accept')} bind:checked={draftAccepted} />
        {/if}
        <!-- Said where the credentials are typed: they go in and the contract answers `config`
             without them, so there is nothing to come back and check later. -->
        <p class="quiet small">{t('app.backup.credentials_write_only')}</p>
        <div>
          <Button
            type="submit"
            tone="primary"
            isBusy={isWorking}
            busyLabel={t('app.backup.adding')}
            disabledReason={insecure.length > 0 && !draftAccepted ? t('app.backup.insecure_first') : undefined}
          >
            {t('app.backup.add')}
          </Button>
        </div>
      </Stack>
    </form>

<style>
  .section {
    margin: 0;
    font-family: var(--font-display);
    font-size: var(--fs-200);
    font-weight: var(--fw-semibold);
  }

  .quiet { margin: 0; color: var(--text-secondary); }

  .small { font-size: var(--fs-075); }

  /* A form on a surface is still a form: its fields keep a measure while the lists and tables of
     the screen take the region (ADR-0065 decision 2). */
  form.panel { max-inline-size: 52ch; }

  .panel {
    padding: var(--sp-200);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-lg);
    background: var(--bg-surface);
    box-shadow: var(--shadow-raised);
  }

  .reasons { margin: 0; padding-inline-start: var(--sp-200); }
</style>
