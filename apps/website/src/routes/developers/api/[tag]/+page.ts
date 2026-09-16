// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// One page per tag, every one of them prerendered: `entries` names them from the document, so a
// tag added to the contract is a page added to the site with no route written for it.
import { error } from '@sveltejs/kit';

import { reference } from '$lib/api/document.ts';

import type { EntryGenerator, PageLoad } from './$types';

export const entries: EntryGenerator = () => reference.tags.map((tag) => ({ tag: tag.slug }));

export const load: PageLoad = ({ params }) => {
  const tag = reference.tags.find((candidate) => candidate.slug === params.tag);
  if (!tag) error(404, 'No such tag');
  return { tag };
};
