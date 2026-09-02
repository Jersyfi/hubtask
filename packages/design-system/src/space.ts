// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// The space scale, as a type, read from the generated tokens rather than typed out again.
//
// A hand-written union would be a second list of the steps, and the next step added to
// tokens.json would be a step the primitives silently refuse (ADR-0029). `keyof typeof` over the
// generated module keeps the compiler's list and the source's list the same list.

/** A step of the space scale: `'025'` … `'1000'`. Nothing else compiles. */
export type Space = keyof (typeof import('../dist/tokens.ts'))['primitive']['space'];

/** How a flex line packs its items across the cross axis. */
export type Align = 'start' | 'center' | 'end' | 'stretch' | 'baseline';

/** …and along the main axis. `between` is the only distribution wave 1 has asked for. */
export type Justify = 'start' | 'center' | 'end' | 'between';
