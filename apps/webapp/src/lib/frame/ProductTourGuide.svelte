<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A product's tour in the frame (user-experience.md §3.4): the first visit of a product's home
  // points once at its create act, the first of three steps; the starter and the base tour come
  // first (§3.1), and neither opens beside it.

  import { untrack } from 'svelte';
  import { Tour } from '@hubtask/design-system/components';

  import { actor } from '../data/account.svelte.ts';
  import { t } from '../i18n/i18n.svelte.ts';
  import { PRODUCTS, productOf } from '../navigation.ts';
  import { onboarding } from '../onboarding/onboarding.svelte.ts';
  import { homeModuleOf, marksFirstVisit } from '../product/producttour.ts';
  import { productTour } from '../product/producttour.svelte.ts';
  import { tour } from '../tour.svelte.ts';

  interface Props {
    /** The route the frame is drawing, whose product home's first visit is marked. */
    routeName: string | null | undefined;
    /** The home's tab the address names (`?tab=`): a tab other than the first is no first visit. */
    routeTab?: string;
    onnavigate: (path: string) => void;
  }

  const { routeName, routeTab, onnavigate }: Props = $props();

  $effect(() => {
    productTour.attach(onnavigate);
  });

  $effect(() => {
    const module = homeModuleOf(routeName, PRODUCTS, routeTab);
    const marks = marksFirstVisit(module, actor.account, {
      baseTourOpen: tour.isOpen,
      baseTourMayStart: onboarding.tourMayStart,
      baseTourTaken: Boolean(actor.account?.onboarding_completed_at),
    });
    if (!module || !marks) return;
    untrack(() => {
      if (!productTour.isOpen && !productTour.wasShown(module)) void productTour.start(module);
    });
  });

  const step = $derived(productTour.step);
  const product = $derived(productTour.module ? productOf(productTour.module) : undefined);
</script>

{#if step && product}
  <Tour
    target={productTour.target}
    caption={t(`app.product_tour.${step.id}.caption`, { product: t(product.code) })}
    body={t(`app.product_tour.${step.id}.body`, { product: t(product.code) })}
    countLabel={t('app.tour.count', { number: String(productTour.index + 1), of: String(productTour.count) })}
    nextLabel={productTour.isLast ? t('app.product_tour.done') : t('app.tour.next')}
    backLabel={t('app.tour.back')}
    skipLabel={t('app.product_tour.skip')}
    hasBack={productTour.index > 0}
    placement={{ side: step.side, align: 'start' }}
    onNext={() => void productTour.next()}
    onBack={() => void productTour.back()}
    onSkip={() => void productTour.skip()}
  />
{/if}
