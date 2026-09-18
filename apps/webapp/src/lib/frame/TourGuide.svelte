<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The tour in the frame (F6-14): the design system's pattern over the real interface, the
  // words for each step resolved here, and the start decided once the account has arrived. The
  // frame mounts this on every route because the tour walks routes.

  import { Tour } from '@hubtask/design-system/components';

  import { actor } from '../data/account.svelte.ts';
  import { t } from '../i18n/i18n.svelte.ts';
  import { tour } from '../tour.svelte.ts';

  interface Props {
    onnavigate: (path: string) => void;
  }

  const { onnavigate }: Props = $props();

  $effect(() => {
    tour.attach(onnavigate);
  });

  // The first sign-in of an account that has not taken the tour: started once, when the account
  // is known. A second arrival of the same account - a re-read after a preference changed - does
  // not start it again, and neither does a tab that already walked it.
  let startedFor = $state<string | undefined>(undefined);
  $effect(() => {
    const account = actor.account;
    if (!account || startedFor === account.id || tour.isOpen) return;
    if (!tour.shouldStart()) return;
    startedFor = account.id;
    void tour.start();
  });

  const step = $derived(tour.step);
</script>

{#if step}
  <Tour
    target={tour.target}
    caption={t(`app.tour.${step.id}.caption`)}
    body={t(`app.tour.${step.id}.body`)}
    countLabel={t('app.tour.count', { number: String(tour.index + 1), of: String(tour.count) })}
    nextLabel={tour.isLast ? t('app.tour.make_one') : t('app.tour.next')}
    backLabel={t('app.tour.back')}
    skipLabel={t('app.tour.skip')}
    hasBack={tour.index > 0}
    placement={{ side: step.side, align: 'start' }}
    onNext={() => void tour.next()}
    onBack={() => void tour.back()}
    onSkip={() => void tour.skip()}
  />
{/if}
