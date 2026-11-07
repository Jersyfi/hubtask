// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The reactive half of `DiscardModel`: a dialog that holds a form creates one with what counts as
 * a change and its own close, hands `guard.dismiss` to `Dialog`'s `onDismiss`, and draws
 * `DiscardDialog` beside it.
 */

import { DiscardModel } from './discard.ts';

export class DiscardGuard {
  #model: DiscardModel;
  #isAsking = $state(false);
  readonly #close: () => void;

  constructor(hasChanges: () => boolean, close: () => void) {
    this.#model = new DiscardModel(hasChanges);
    this.#close = close;
  }

  get isAsking(): boolean {
    return this.#isAsking;
  }

  readonly dismiss = (): boolean => {
    const mayClose = this.#model.dismiss();
    this.#isAsking = this.#model.isAsking;
    return mayClose;
  };

  readonly keep = (): void => {
    this.#model.keep();
    this.#isAsking = false;
  };

  readonly discard = (): void => {
    this.#model.discard(this.#close);
    this.#isAsking = false;
  };
}
