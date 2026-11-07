// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * A long list that arrived whole, shown as the device's rows per page and continued at its foot
 * (page-system PS-8, D-1): *Showing 25 of 87*, *Show 25 more*, rows per page 25 · 50 · 100. No
 * page numbers; narrowing the list starts it again at its first rows.
 */

import { device } from '../device.svelte.ts';
import { t } from '../i18n/i18n.svelte.ts';
import { rowsPerPageOf } from './list.ts';

export class Continued<T> {
  #rows: () => readonly T[];
  #extra = $state(0);

  /** `rows` is the narrowed list, read again whenever it changes. */
  constructor(rows: () => readonly T[]) {
    this.#rows = rows;
  }

  #limit(): number {
    return device.rowsPerPage + this.#extra;
  }

  /** The rows on show. */
  get shown(): readonly T[] {
    return this.#rows().slice(0, this.#limit());
  }

  /** Back to the first rows: after the search or a filter changed. */
  restart(): void {
    this.#extra = 0;
  }

  /** The foot's words: *Showing 25 of 87*, or *Showing 9* where everything is on show. */
  get shownLabel(): string {
    const all = this.#rows().length;
    const shown = Math.min(all, this.#limit());
    return shown < all ? t('app.page.showing_of', { shown: String(shown), count: String(all) }) : t('app.page.showing', { count: String(all) });
  }

  /** `DataTable`'s `more`, while rows are left. */
  get more() {
    const left = this.#rows().length - this.#limit();
    if (left <= 0) return undefined;
    return {
      label: t('app.page.show_more', { count: String(Math.min(left, device.rowsPerPage)) }),
      hasMore: true,
      onLoadMore: () => {
        this.#extra += device.rowsPerPage;
      },
    };
  }

  /** `DataTable`'s `perPage`, once the list is longer than the shortest page. */
  get perPage() {
    if (this.#rows().length <= rowsPerPageOf(undefined)) return undefined;
    return {
      label: t('app.page.rows_per_page'),
      value: device.rowsPerPage,
      onchange: (value: number) => {
        device.setRowsPerPage(rowsPerPageOf(String(value)));
        this.#extra = 0;
      },
    };
  }
}
