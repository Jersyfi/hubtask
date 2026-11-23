<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // *Logo and app bar*, the group under the workspace's name (UC-INS-22 checks 1 to 4, 10 and 11;
  // the approved draft's scene 1, decision G5): a preview of the header on a light and on a dark
  // surface side by side, the choice of what it shows, then the three pictures. The choice is saved
  // with the section's save bar; a picture is set by choosing it.
  //
  // **The preview is live.** It draws the choice as it stands in the form and a file as soon as it
  // is chosen, before the server has it - the row beside it says that it is still going.
  //
  // **Where a level above switched it off**, the group is one row: no control, and who decided,
  // as *Private hubs allowed* says it (check 11). An "off" above cannot be switched on here; one this
  // workspace set is switched on again under *Sign-in → Own logo and name*, and the row links there.

  import { untrack } from 'svelte';

  import { values } from '@hubtask/design-system';
  import { Radio, SettingRow } from '@hubtask/design-system/components';
  import type { WorkspaceBranding, WorkspaceHeader, WorkspacePicture } from '@hubtask/sync-engine';

  import { branding as store, type PictureLook, type PictureRead } from '../data/branding.svelte.ts';
  import { manifest } from '../data/capabilities.svelte.ts';
  import { live } from '../data/live.svelte.ts';
  import { pictureLimitOf } from '../data/media.ts';
  import type { Setting } from '../data/signinpolicy.svelte.ts';
  import { formatBytes } from '../i18n/bytes.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import BrandingPicture, { type PictureLine } from './BrandingPicture.svelte';
  import { contrastOf, HEADERS, mayShowLogoAlone, pictureOf, PICTURES, type PictureSource, type Surface } from './branding.ts';
  import HeaderPreview from './HeaderPreview.svelte';
  import ValueSource from './ValueSource.svelte';

  interface Props {
    branding: WorkspaceBranding;
    /** The workspace's name as typed in the row above. */
    name: string;
    /** The choice as the form holds it. */
    header: WorkspaceHeader;
    isHeaderChanged: boolean;
    /** The server's refusal of the choice, said at the choice. */
    headerError?: string;
    onheader: (header: WorkspaceHeader) => void;
    /** The switch above, for who switched it off. */
    allowedBy?: Setting<boolean>;
    /** Whether a picture is going: the leave guard holds while one is. */
    isUploading?: boolean;
  }

  let { branding, name, header, isHeaderChanged, headerError, onheader, allowedBy, isUploading = $bindable(false) }: Props = $props();

  const SURFACE_HEX: Record<Surface, string> = { light: values.light.bg.surface, dark: values.dark.bg.surface };

  /** A file on its way, by picture: the preview draws it before the server has it. */
  let staged = $state<Partial<Record<WorkspacePicture, PictureLook>>>({});
  $effect(() => {
    const going = Object.keys(staged).length > 0;
    untrack(() => (isUploading = going));
  });

  const stored = $derived(Object.fromEntries(PICTURES.flatMap((picture) => {
    const id = pictureOf(branding, picture);
    return id ? [[picture, id]] : [];
  })) as Partial<Record<WorkspacePicture, string>>);
  /** Where each stored picture's bytes stand: being read, drawn, or unreadable and why. */
  const reads = $derived(Object.fromEntries(PICTURES.flatMap((picture) => {
    const read = store.read(stored[picture]);
    return read ? [[picture, read]] : [];
  })) as Partial<Record<WorkspacePicture, PictureRead>>);
  const looks = $derived(Object.fromEntries(PICTURES.flatMap((picture) => {
    const read = reads[picture];
    const look = staged[picture] ?? (read?.status === 'ready' ? read.look : undefined);
    return look ? [[picture, look]] : [];
  })) as Partial<Record<WorkspacePicture, PictureLook>>);
  /** What exists or is going: the preview's pictures and what the choice may offer. */
  const present = $derived({ ...stored, ...Object.fromEntries(Object.keys(staged).map((picture) => [picture, 'staged'])) } as Partial<Record<WorkspacePicture, string>>);
  /** A stored picture that cannot be drawn here, and is not being replaced. */
  const isUnreadable = (picture: WorkspacePicture) => !staged[picture] && reads[picture]?.status === 'unreadable';
  /**
   * Each present picture's local address, `null` until its bytes are in. One that cannot be drawn
   * here is left out, so the preview falls back past it by G8 - and says so.
   */
  const sources = $derived(Object.fromEntries(PICTURES.flatMap((picture) => (present[picture] && !isUnreadable(picture) ? [[picture, looks[picture]?.url ?? null]] : []))) as Partial<Record<WorkspacePicture, PictureSource>>);
  const isFallingBack = $derived(PICTURES.some(isUnreadable));
  const shown = $derived(header === 'LOGO' && !mayShowLogoAlone(present) ? 'PRODUCT' : header);

  const limit = $derived(pictureLimitOf(manifest.value));
  const offline = $derived(live.state === 'offline' ? t('app.branding.offline') : undefined);

  const options = $derived(HEADERS.map((each) => ({
    value: each,
    label: t(`app.branding.header_${each}`),
    ...(each === 'LOGO' && !mayShowLogoAlone(present)
      ? { disabledReason: t('app.branding.logo_needs_picture') }
      : { hint: t(`app.branding.header_${each}_hint`, { name }) }),
  })));

  const verdict = (picture: WorkspacePicture, surface: Surface) => {
    const pixels = looks[picture]?.pixels;
    return pixels ? contrastOf(pixels, SURFACE_HEX[surface]) : undefined;
  };

  /** What each row says under its picture. */
  const lines = $derived.by((): Partial<Record<WorkspacePicture, PictureLine>> => {
    const light = verdict('LOGO_LIGHT', 'light');
    const dark = looks.LOGO_DARK ? verdict('LOGO_DARK', 'dark') : undefined;
    const lightOnDark = !looks.LOGO_DARK && looks.LOGO_LIGHT ? verdict('LOGO_LIGHT', 'dark') : undefined;
    const symbol = [verdict('SYMBOL', 'light'), verdict('SYMBOL', 'dark')] as const;
    const symbolHard = symbol[0] === 'hard' && symbol[1] === 'hard' ? 'both' : symbol[0] === 'hard' ? 'light' : symbol[1] === 'hard' ? 'dark' : undefined;
    const status = Object.fromEntries(PICTURES.flatMap((picture): [WorkspacePicture, PictureLine][] => {
      const read = reads[picture];
      if (staged[picture] || !read || read.status === 'ready') return [];
      return [[picture, read.status === 'reading'
        ? { tone: 'quiet', text: t('app.branding.picture_reading') }
        : { tone: 'hard', text: t(`app.branding.unreadable_${read.failure}`) }]];
    }));
    return {
      ...(light ? { LOGO_LIGHT: { tone: light, text: t(light === 'reads' ? 'app.branding.reads_light' : 'app.branding.hard_logo_light') } } : {}),
      ...(dark ? { LOGO_DARK: { tone: dark, text: t(dark === 'reads' ? 'app.branding.reads_dark' : 'app.branding.hard_logo_dark') } } : {}),
      ...(lightOnDark === 'hard' ? { LOGO_DARK: { tone: 'hard' as const, text: t('app.branding.hard_logo_dark') } } : {}),
      ...(symbolHard
        ? { SYMBOL: { tone: 'hard' as const, text: t(`app.branding.hard_symbol_${symbolHard}`) } }
        : symbol[0] && symbol[1] ? { SYMBOL: { tone: 'reads' as const, text: t('app.branding.reads_both') } } : {}),
      ...status,
    };
  });

  const uid = $props.id();

  // A refused choice is said at the choice, and the focus goes there (design-system.md §4, case 3).
  let choiceRoot = $state<HTMLElement | undefined>(undefined);
  $effect(() => {
    if (headerError) untrack(() => choiceRoot?.querySelector<HTMLInputElement>('input:checked')?.focus());
  });
  const previewId = `branding-preview-${uid}`;
  const isLocked = $derived(!branding.allowed);
  /** Who switched it off: the lock's level, else the level whose value holds. */
  const origin = $derived(allowedBy ? (allowedBy.lock ?? allowedBy.source) : undefined);
</script>

<div class="branding" data-branding>
  <div class="group-label">{t('app.branding.group')}</div>
  {#if isLocked}
    <!-- Who decided is said once, by the chip, as *Private hubs allowed* says it; its details say
         whom to ask. Where this workspace switched it off, the row points to its switch. -->
    <div class="locked" data-branding-locked={origin}>
      <SettingRow label={t('app.branding.own')} help={t('app.branding.locked_hint')}>
        {#snippet source()}
          {#if allowedBy}<ValueSource source={allowedBy.source} lock={allowedBy.lock} value={t('app.branding.locked_value')} />{/if}
        {/snippet}
        {#snippet control()}
          <span class="locked-value">
            <span>{t('app.branding.locked_value')}</span>
            {#if origin === 'WORKSPACE'}<a href="/administration/sign-in/own-branding">{t('app.branding.switch_on_at')}</a>{/if}
          </span>
        {/snippet}
      </SettingRow>
    </div>
  {:else}
    <div class="preview">
      <p class="preview-head" id={previewId}>{t('app.branding.preview')}</p>
      <div class="pair" role="group" aria-labelledby={previewId}>
        {#each ['light', 'dark'] as const as surface (surface)}
          <HeaderPreview {surface} header={shown} pictures={sources} {name} caption={t(`app.branding.preview_${surface}`)} />
        {/each}
      </div>
      {#if isFallingBack}<p class="fallback" data-branding-fallback>{t('app.branding.preview_fallback')}</p>{/if}
    </div>
    <div class="row" data-branding-header>
      <SettingRow
        label={t('app.branding.header')}
        help={t('app.branding.header_hint')}
        fieldWidth="full"
        isChanged={isHeaderChanged}
        reset={shown !== 'PRODUCT' ? { label: t('app.branding.reset'), onclick: () => onheader('PRODUCT') } : undefined}
      >
        {#snippet source()}
          <ValueSource source={branding.header === 'PRODUCT' ? 'DEFAULT' : 'WORKSPACE'} value={t(`app.branding.header_${branding.header}`)} />
        {/snippet}
        {#snippet control()}
          <span class="choice" bind:this={choiceRoot}>
            <Radio label={t('app.branding.header')} {options} error={headerError} bind:value={() => shown, (next) => onheader(next as WorkspaceHeader)} />
          </span>
        {/snippet}
      </SettingRow>
    </div>
    {#if offline}<p class="offline" data-branding-offline data-offline-line>{offline}</p>{/if}
    {#each PICTURES as picture (picture)}
      <div class="row" data-branding-picture={picture}>
        <SettingRow
          label={t(`app.branding.picture_${picture}`)}
          help={t(`app.branding.picture_${picture}_hint`, { limit: formatBytes(limit, messages.locale) })}
          fieldWidth="full"
        >
          {#snippet control(nameId)}
            <BrandingPicture
              {picture}
              label={t(`app.branding.picture_${picture}`)}
              {nameId}
              read={reads[picture]}
              standIn={picture === 'LOGO_DARK' && !stored.LOGO_DARK && looks.LOGO_LIGHT ? { look: looks.LOGO_LIGHT, text: t('app.branding.light_applies') } : undefined}
              line={lines[picture]}
              {limit}
              header={branding.header}
              isLast={Object.keys(stored).length === 1 && stored[picture] !== undefined}
              disabledReason={offline}
              onstage={(look) => {
                const { [picture]: _gone, ...rest } = staged;
                staged = look ? { ...rest, [picture]: look } : rest;
              }}
            />
          {/snippet}
        </SettingRow>
      </div>
    {/each}
  {/if}
</div>

<style>
  .group-label {
    padding: var(--sp-150) var(--sp-250) var(--sp-050);
    border-block-start: var(--bw-hairline) solid var(--border-subtle);
    color: var(--text-subtle);
    font-size: var(--fs-075);
    font-weight: var(--fw-semibold);
    text-transform: uppercase;
  }

  .locked-value { display: flex; flex-direction: column; gap: var(--sp-050); }

  .preview-head { margin: 0; padding: var(--sp-150) var(--sp-250) 0; font-weight: var(--fw-medium); }

  .pair { display: grid; grid-template-columns: minmax(0, 1fr) minmax(0, 1fr); gap: var(--sp-150); padding: var(--sp-100) var(--sp-250) var(--sp-200); }

  /* The choice is named by its row; the group's own legend stays for the tree only. */
  .choice { display: block; }
  .choice :global(.legend) {
    position: absolute;
    inline-size: var(--bw-hairline);
    block-size: var(--bw-hairline);
    overflow: hidden;
    clip-path: inset(50%);
    white-space: nowrap;
  }

  .fallback { margin: 0; padding: 0 var(--sp-250) var(--sp-200); color: var(--text-subtle); font-size: var(--fs-075); }

  .offline { margin: 0; padding: var(--sp-100) var(--sp-250); color: var(--text-secondary); font-size: var(--fs-075); }

  .row :global(.setting-row > .setting-line:first-child) { border-block-start: var(--bw-hairline) solid var(--border-subtle); }

  /* design-system-lint-ignore: `primitive.breakpoint.medium` (600px), SettingRow's own; a media query cannot read a custom property. */
  @media (width < 600px) {
    .group-label, .preview-head, .offline { padding-inline: var(--sp-200); }
    .pair { grid-template-columns: minmax(0, 1fr); padding-inline: var(--sp-200); }
  }
</style>
