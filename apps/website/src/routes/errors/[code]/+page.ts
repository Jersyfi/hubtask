// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// One prerendered page per code: `entries` names them from the list, so a code added there is a
// page added to the site with no route written for it.
import { error } from '@sveltejs/kit';

import { codes } from '#lib/errors/codes.ts';

import type { EntryGenerator, PageLoad } from './$types';

export const entries: EntryGenerator = () => codes.map((entry) => ({ code: entry.code }));

export const load: PageLoad = ({ params }) => {
  const entry = codes.find((candidate) => candidate.code === params.code);
  if (!entry) error(404, 'No such error code');
  return { entry };
};
