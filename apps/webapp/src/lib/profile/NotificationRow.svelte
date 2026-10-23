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
  // A row of its own because a reason group belongs to a component: the note is this row's.

  import { Checkbox, provideReasonGroup } from '@hubtask/design-system/components';

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

<tr data-setting={`notify-${category}`}>
  <!-- A phrase per category the catalogue knows, and `humanise` for one it does not: an
       installation that tells people about something newer still reads readably rather than
       showing a key. -->
  <th scope="row" class="what">
    {t(`app.profile.category_${category}`)}
    {#if note}<span class="note" id={noteId}>{note}</span>{/if}
  </th>
  {#each channels as channel (channel)}
    {@const row = preferenceFor(rows, category, channel)}
    <td>
      <Checkbox
        label={t('app.profile.enabled')}
        isLabelHidden
        checked={isAlwaysOn ? true : (row?.enabled ?? true)}
        disabledReason={note}
        isBusy={busy.has(`${channel}:enabled`)}
        busyLabel={t('app.workspace.saving')}
        onchange={(event: Event) => onset(channel, { enabled: (event.currentTarget as HTMLInputElement).checked })}
      />
    </td>
    <td>
      <Checkbox
        label={t('app.profile.include_title')}
        isLabelHidden
        checked={row?.include_title ?? false}
        isBusy={busy.has(`${channel}:include_title`)}
        busyLabel={t('app.workspace.saving')}
        onchange={(event: Event) => onset(channel, { include_title: (event.currentTarget as HTMLInputElement).checked })}
      />
    </td>
  {/each}
</tr>

<style>
  /* The category is the column that grows, so the two questions stand together at the end of the
     row rather than being pushed to opposite edges by a table sharing its width evenly. */
  .what { inline-size: 100%; font-weight: var(--fw-regular); text-align: start; }

  .note { display: block; color: var(--text-secondary); font-size: var(--fs-075); }
</style>
