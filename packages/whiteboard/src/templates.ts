// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * Hubtask's board templates (research §4.3, the *Must* rows): three retrospectives, a Kanban
 * board, a brainstorm and a flowchart. A template is objects put on the board at a point; its
 * words come from the application in the reader's language, by the keys each template names.
 */

import type { Point } from './geometry.ts';
import type { Element } from './scene.ts';

export type TemplateName = 'RETRO_START_STOP_CONTINUE' | 'RETRO_FOUR_LS' | 'RETRO_MAD_SAD_GLAD' | 'KANBAN' | 'BRAINSTORM' | 'FLOWCHART';

export const TEMPLATES: readonly TemplateName[] = ['RETRO_START_STOP_CONTINUE', 'RETRO_FOUR_LS', 'RETRO_MAD_SAD_GLAD', 'KANBAN', 'BRAINSTORM', 'FLOWCHART'];

/** The word keys each template asks for, in order. */
export const TEMPLATE_WORDS: Record<TemplateName, readonly string[]> = {
  RETRO_START_STOP_CONTINUE: ['start', 'stop', 'continue'],
  RETRO_FOUR_LS: ['liked', 'learned', 'lacked', 'longed_for'],
  RETRO_MAD_SAD_GLAD: ['mad', 'sad', 'glad'],
  KANBAN: ['to_do', 'doing', 'done'],
  BRAINSTORM: ['topic', 'idea'],
  FLOWCHART: ['begin', 'step', 'decision', 'yes', 'no', 'finish'],
};

const COLUMN = { width: 420, height: 720, gap: 40 };
const RETRO_COLOURS = ['green', 'red', 'blue', 'amber'];

export interface TemplateInput {
  /** The board point the template is centred on. */
  at: Point;
  words: (key: string) => string;
  newId: () => string;
  /** For a Kanban board: entries put as cards into its first column. */
  entries?: readonly string[];
}

/** The objects of a template, centred on a point. */
export function template(name: TemplateName, input: TemplateInput): Element[] {
  switch (name) {
    case 'RETRO_START_STOP_CONTINUE':
    case 'RETRO_FOUR_LS':
    case 'RETRO_MAD_SAD_GLAD':
      return columns(TEMPLATE_WORDS[name], input, true);
    case 'KANBAN':
      return columns(TEMPLATE_WORDS.KANBAN, input, false);
    case 'BRAINSTORM':
      return brainstorm(input);
    case 'FLOWCHART':
      return flowchart(input);
  }
}

function columns(keys: readonly string[], input: TemplateInput, notes: boolean): Element[] {
  const total = keys.length * COLUMN.width + (keys.length - 1) * COLUMN.gap;
  const left = Math.round(input.at.x - total / 2);
  const top = Math.round(input.at.y - COLUMN.height / 2);
  const out: Element[] = [];
  keys.forEach((key, i) => {
    const x = left + i * (COLUMN.width + COLUMN.gap);
    out.push({ id: input.newId(), kind: 'FRAME', x, y: top, width: COLUMN.width, height: COLUMN.height, text: input.words(key) });
    if (notes) {
      out.push({ id: input.newId(), kind: 'STICKY', x: x + 40, y: top + 40, width: 160, height: 160, text: '', color_token: RETRO_COLOURS[i % RETRO_COLOURS.length] });
    }
  });
  if (!notes && input.entries) {
    const x = left + 20;
    input.entries.slice(0, 5).forEach((id, i) => {
      out.push({ id: input.newId(), kind: 'CARD', x, y: top + 40 + i * 128, width: COLUMN.width - 40, height: 112, target: { kind: 'ENTRY', id } });
    });
  }
  return out;
}

function brainstorm(input: TemplateInput): Element[] {
  const topic: Element = {
    id: input.newId(), kind: 'SHAPE', shape: 'ELLIPSE', x: Math.round(input.at.x - 120), y: Math.round(input.at.y - 70),
    width: 240, height: 140, text: input.words('topic'),
  };
  const out: Element[] = [topic];
  const radius = 360;
  for (let i = 0; i < 6; i++) {
    const angle = (i / 6) * 2 * Math.PI - Math.PI / 2;
    const note: Element = {
      id: input.newId(), kind: 'STICKY', x: Math.round(input.at.x + radius * Math.cos(angle) - 80), y: Math.round(input.at.y + radius * Math.sin(angle) - 80),
      width: 160, height: 160, text: input.words('idea'), color_token: 'amber',
    };
    out.push(note, { id: input.newId(), kind: 'CONNECTOR', from: topic.id, to: note.id, route: 'CURVED', start: 'NONE', end: 'NONE', x: 0, y: 0, width: 0, height: 0 });
  }
  return out;
}

function flowchart(input: TemplateInput): Element[] {
  const { at, words, newId } = input;
  const node = (shape: Element['shape'], dx: number, dy: number, key: string, width = 200, height = 100): Element => ({
    id: newId(), kind: 'SHAPE', shape, x: Math.round(at.x + dx - width / 2), y: Math.round(at.y + dy - height / 2), width, height, text: words(key),
  });
  const link = (from: Element, to: Element, label = ''): Element => ({
    id: newId(), kind: 'CONNECTOR', from: from.id, to: to.id, route: 'ELBOW', start: 'NONE', end: 'ARROW', x: 0, y: 0, width: 0, height: 0, text: label,
  });
  const begin = node('ELLIPSE', 0, -360, 'begin', 180, 90);
  const step = node('RECTANGLE', 0, -180, 'step');
  const decision = node('DIAMOND', 0, 20, 'decision', 220, 140);
  const yes = node('RECTANGLE', 0, 220, 'step');
  const finish = node('ELLIPSE', 0, 400, 'finish', 180, 90);
  const no = node('RECTANGLE', 340, 20, 'step');
  return [
    begin, step, decision, yes, finish, no,
    link(begin, step), link(step, decision), link(decision, yes, words('yes')), link(yes, finish), link(decision, no, words('no')), link(no, step),
  ];
}
