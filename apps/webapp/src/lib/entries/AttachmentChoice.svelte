<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A FILE field's picker: one of the entry's own attachments (UC-WRK-12). Its own component so the
  // attachment list is read only where such a field is drawn; the server holds the choice to what
  // the entry carries (`fields.value_not_an_attachment`).

  import { untrack } from 'svelte';
  import { Select } from '@hubtask/design-system/components';

  import { attachmentsPath, type MediaPage } from '../data/media.svelte.ts';
  import { resource } from '../data/resource.svelte.ts';
  import { t } from '../i18n/i18n.svelte.ts';

  interface Props {
    itemId: string;
    value: unknown;
    onchoose: (mediaId: string | null) => void;
  }

  const { itemId, value, onchoose }: Props = $props();

  const page = resource<MediaPage>({ path: untrack(() => attachmentsPath(itemId)) });
  const files = $derived(page.state.status === 'ready' ? (page.state.data.data ?? []) : []);
  const options = $derived.by(() => {
    const listed = [
      { value: '', label: t('app.fields.pick_file_none') },
      ...files.map((file) => ({ value: file.id, label: file.file_name ?? file.id })),
    ];
    // A file detached since it was chosen stays chosen, said to be out of reach.
    if (typeof value === 'string' && value !== '' && !files.some((file) => file.id === value)) {
      listed.push({ value, label: t('app.links.unavailable') });
    }
    return listed;
  });
</script>

<Select
  label={t('app.fields.pick_file')}
  size="sm"
  {options}
  value={typeof value === 'string' ? value : ''}
  hint={page.state.status === 'ready' && files.length === 0 ? t('app.fields.pick_file_empty') : undefined}
  onchange={(event) => onchoose((event.currentTarget as HTMLSelectElement).value || null)}
/>
