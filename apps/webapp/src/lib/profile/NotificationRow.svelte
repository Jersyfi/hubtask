<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // One category of what a person is told about, across the channels: per channel, whether it is
  // sent and whether it names the entry.
  //
  // **A category nobody can switch off says so once, for the row** (UC-NOT-03 check 4, UC-WRK-32
  // check 4): a note under its name. Its *Tell me* boxes stay - they show that it is sent - and
  // share the note's reason through the design system's reason group, so each is refused, is
  // described by the note, and draws no bubble of its own. The sentence used to stand under every
  // one of them, in a column kept wide for it.
  //
  // A row of its own because a reason group belongs to a component: the note is this row's. A
  // `SettingRow` (page-system PS-11): the category's name at the start, its boxes at the control's
  // place, so every row's boxes stand at one x.

  import { Checkbox, SettingRow, provideReasonGroup } from '@hubtask/design-system/components';

  import { preferenceFor } from '../data/preferences.ts';
  import type { NotificationPreference } from '@hubtask/sync-engine';
  import { t } from '../i18n/i18n.svelte.ts';

  type Row = NotificationPreference;

  interface Props {
    category: string;
    channels: readonly string[];
    rows: readonly Row[];
    /** Whether this category is always sent, whatever the preference says. */
    isAlwaysOn: boolean;
    /** Which of this row's boxes are being written, as `channel:field`. */
    busy: ReadonlySet<string>;
    onset: (channel: string, next: { enabled?: boolean; include_title?: boolean }) => void;
  }

  const { category, channels, rows, isAlwaysOn, busy, onset }: Props = $props();

  const instanceId = $props.id();
  const noteId = `notification-${instanceId}-note`;
  const note = $derived(isAlwaysOn ? t('app.profile.always_on_note') : undefined);
  provideReasonGroup(() => ({ id: noteId, reason: note }));
</script>

<div data-setting={`notify-${category}`} role="group" aria-label={t(`app.profile.category_${category}`)}>
  <!-- A phrase per category the catalogue knows: an installation that tells people about something
       newer still reads readably rather than showing a key. -->
  <SettingRow label={t(`app.profile.category_${category}`)} fieldWidth="lg">
    {#snippet control()}
      <span class="boxes">
        {#each channels as channel (channel)}
          {@const row = preferenceFor(rows, category, channel)}
          <Checkbox
            label={channels.length > 1 ? `${t(`app.profile.channel_${channel}`)} · ${t('app.profile.enabled')}` : t('app.profile.enabled')}
            checked={isAlwaysOn ? true : (row?.enabled ?? true)}
            disabledReason={note}
            isBusy={busy.has(`${channel}:enabled`)}
            busyLabel={t('app.workspace.saving')}
            onchange={(event: Event) => onset(channel, { enabled: (event.currentTarget as HTMLInputElement).checked })}
          />
          <Checkbox
            label={channels.length > 1 ? `${t(`app.profile.channel_${channel}`)} · ${t('app.profile.include_title')}` : t('app.profile.include_title')}
            checked={row?.include_title ?? false}
            isBusy={busy.has(`${channel}:include_title`)}
            busyLabel={t('app.workspace.saving')}
            onchange={(event: Event) => onset(channel, { include_title: (event.currentTarget as HTMLInputElement).checked })}
          />
        {/each}
        {#if note}<span class="note" id={noteId}>{note}</span>{/if}
      </span>
    {/snippet}
  </SettingRow>
</div>

<style>
  /* Two boxes per channel in fixed columns, so every row's boxes stand at the same x. */
  .boxes { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: var(--sp-100) var(--sp-250); align-items: center; }

  .note { grid-column: 1 / -1; color: var(--text-secondary); font-size: var(--fs-075); }
</style>
