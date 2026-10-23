<!--
  What a change of structure would stop showing, before it is made (UC-WRK-24 checks 2, 6, 7): the
  levels with how many entries each, the values on entries that stay shown, the views, what would
  pause, and for a hub the collections that keep their own value. *Hide* is the default and undone
  by switching back; *Flatten* moves the hidden entries to the top level and says it is not.
-->
<script lang="ts">
  import { Button, Dialog, Inline, Radio, Stack } from '@hubtask/design-system/components';
  import type { StructurePreview, StructureSettings } from '@hubtask/sync-engine';

  import { levelWords } from '../data/levels.ts';
  import { t } from '../i18n/i18n.svelte.ts';

  interface Props {
    preview: StructurePreview;
    /** The structure in force now: the levels keep the names they have before the change. */
    settings: StructureSettings | undefined;
    isSaving: boolean;
    onconfirm: (narrowing: 'HIDE' | 'FLATTEN') => void;
    oncancel: () => void;
  }

  let { preview, settings, isSaving, onconfirm, oncancel }: Props = $props();

  let isOpen = $state(true);
  let narrowing = $state<'HIDE' | 'FLATTEN'>('HIDE');

  const flattens = $derived(preview.flatten.entries);
</script>

<Dialog
  bind:isOpen
  title={t('app.structure.narrowing.title')}
  dismissLabel={t('app.workspace.cancel')}
  onClose={oncancel}
>
  <div class="narrowing" data-narrowing-dialog>
    <Stack gap="200">
      <p class="hint">{t('app.structure.narrowing.intro')}</p>
      <ul class="lost">
        {#each preview.levels as level (level.type)}
          <li data-narrowed-level={level.type}>
            {t('app.structure.narrowing.level', { level: levelWords(settings, level.type, t).label, count: String(level.entries) })}
          </li>
        {/each}
        {#each preview.fields as field (`${field.type}/${field.capability}`)}
          <li data-narrowed-field={field.capability}>
            {t('app.structure.narrowing.field', {
              field: t(`app.structure.field.${field.capability}`),
              level: levelWords(settings, field.type, t).label,
              count: String(field.entries),
            })}
          </li>
        {/each}
        {#each preview.views as view (view)}
          <li data-narrowed-view={view}>{t('app.structure.narrowing.view', { view: t(`app.view.${view}`) })}</li>
        {/each}
        {#if preview.automation.length > 0}
          <li>{t('app.structure.narrowing.automation', { count: String(preview.automation.length) })}</li>
        {/if}
      </ul>
      {#if preview.keeping.length > 0}
        <p class="hint">{t('app.structure.narrowing.keeping_intro')}</p>
        <ul class="lost">
          {#each preview.keeping as kept (kept.container_id)}
            <li data-keeping={kept.container_id}>
              {t('app.structure.narrowing.keeping', {
                name: kept.name,
                settings: kept.settings.map((setting) => t(`app.structure.setting.${setting}`)).join(', '),
              })}
            </li>
          {/each}
        </ul>
      {/if}
      <Radio
        label={t('app.structure.narrowing.choice')}
        options={[
          { value: 'HIDE', label: t('app.structure.narrowing.hide'), hint: t('app.structure.narrowing.hide_hint') },
          {
            value: 'FLATTEN',
            label: t('app.structure.narrowing.flatten'),
            hint: t('app.structure.narrowing.flatten_hint', { count: String(flattens) }),
            disabledReason: flattens === 0 ? t('app.structure.narrowing.flatten_nothing') : undefined,
          },
        ]}
        bind:value={() => narrowing, (choice) => (narrowing = choice === 'FLATTEN' ? 'FLATTEN' : 'HIDE')}
      />
    </Stack>
  </div>
  {#snippet actions()}
    <Inline gap="100">
      <Button tone="secondary" onclick={() => { isOpen = false; oncancel(); }}>{t('app.workspace.cancel')}</Button>
      <Button tone="primary" onclick={() => onconfirm(narrowing)} isBusy={isSaving} data-narrowing-confirm>
        {t(narrowing === 'FLATTEN' ? 'app.structure.narrowing.confirm_flatten' : 'app.structure.narrowing.confirm_hide')}
      </Button>
    </Inline>
  {/snippet}
</Dialog>

<style>
  .narrowing { max-width: 64ch; }
  .hint { margin: 0; color: var(--text-secondary); }
  .lost { margin: 0; padding-inline-start: var(--sp-300); display: grid; gap: var(--sp-050); }
</style>
