// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * People as the options of a `MultiSelect` - a group's members to add, a page's readers: sorted by
 * name under the reader's locale, each with the address the people list shows beside the name as
 * its second line, so two people of one name are told apart and the list's filter finds an
 * address too (the filter reads the line). No address where the reader may not read it.
 */

import type { ChoiceOption } from '@hubtask/design-system/components';

import { byName } from '../i18n/collation.ts';

export interface PersonWords {
  /** The name, or what is said of somebody unnamed. */
  readonly nameOf: (id: string) => string;
  /** The address where the reader may read it. */
  readonly addressOf: (id: string) => string | undefined;
  readonly locale: string;
}

/**
 * The options for `ids`, by name; `first` (a locked reader) stands before the rest whatever its
 * name, and `extra` adds what one option carries besides (its own label, its reason).
 */
export function personOptions(
  ids: readonly string[],
  words: PersonWords,
  { first, extra }: { readonly first?: string; readonly extra?: (id: string) => Partial<ChoiceOption> } = {},
): ChoiceOption[] {
  const order = byName(words.locale);
  const options = [...new Set(ids)].map((id): ChoiceOption => {
    const address = words.addressOf(id);
    return { value: id, label: words.nameOf(id), ...(address ? { hint: address } : {}), ...extra?.(id) };
  });
  const sorted = options.filter((each) => each.value !== first).sort((left, right) => order(left.label, right.label) || order(left.hint ?? '', right.hint ?? ''));
  const lead = options.find((each) => each.value === first);
  return lead ? [lead, ...sorted] : sorted;
}
