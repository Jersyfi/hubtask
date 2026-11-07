// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * `:shortcode:` typed in a page becomes its emoji. The page keeps the Unicode character, so the
 * Markdown, search and every other reader see the emoji itself, not a code only this editor knows.
 *
 * The table is kept by hand, the codes the common chat tools share, so typing one needs no data
 * loaded; the full set is the application's emoji picker's. An unknown code stays the text typed.
 */

import { Extension, InputRule } from '@tiptap/core';

export const EMOJI_SHORTCODES: Readonly<Record<string, string>> = Object.freeze({
  '+1': '👍', '-1': '👎', thumbsup: '👍', thumbsdown: '👎', ok_hand: '👌', clap: '👏', wave: '👋', pray: '🙏',
  muscle: '💪', raised_hands: '🙌', point_right: '👉', point_left: '👈', point_up: '☝️', eyes: '👀',
  smile: '😄', smiley: '😃', grin: '😁', laughing: '😆', joy: '😂', rofl: '🤣', wink: '😉', blush: '😊',
  slightly_smiling_face: '🙂', upside_down_face: '🙃', heart_eyes: '😍', thinking: '🤔', neutral_face: '😐',
  confused: '😕', worried: '😟', cry: '😢', sob: '😭', angry: '😠', rage: '😡', scream: '😱', sweat_smile: '😅',
  sunglasses: '😎', nerd_face: '🤓', partying_face: '🥳', zipper_mouth_face: '🤐', shushing_face: '🤫',
  facepalm: '🤦', shrug: '🤷', sleeping: '😴', mask: '😷', exploding_head: '🤯',
  heart: '❤️', broken_heart: '💔', sparkling_heart: '💖', star: '⭐', sparkles: '✨', fire: '🔥', boom: '💥',
  zap: '⚡', tada: '🎉', confetti_ball: '🎊', gift: '🎁', trophy: '🏆', medal: '🏅', rocket: '🚀', dart: '🎯',
  bulb: '💡', memo: '📝', pencil: '✏️', book: '📖', books: '📚', bookmark: '🔖', paperclip: '📎', pushpin: '📌',
  calendar: '📅', date: '📅', clock: '🕐', hourglass: '⌛', alarm_clock: '⏰', stopwatch: '⏱️',
  email: '📧', envelope: '✉️', phone: '📞', computer: '💻', keyboard: '⌨️', printer: '🖨️', mag: '🔍',
  lock: '🔒', unlock: '🔓', key: '🔑', link: '🔗', gear: '⚙️', wrench: '🔧', hammer: '🔨', tools: '🛠️',
  package: '📦', inbox_tray: '📥', outbox_tray: '📤', chart_with_upwards_trend: '📈',
  chart_with_downwards_trend: '📉', bar_chart: '📊', clipboard: '📋', file_folder: '📁', page_facing_up: '📄',
  white_check_mark: '✅', heavy_check_mark: '✔️', ballot_box_with_check: '☑️', x: '❌', negative_squared_cross_mark: '❎',
  warning: '⚠️', no_entry: '⛔', stop_sign: '🛑', question: '❓', exclamation: '❗', bangbang: '‼️',
  information_source: 'ℹ️', bell: '🔔', no_bell: '🔕', speech_balloon: '💬', thought_balloon: '💭', loudspeaker: '📢',
  red_circle: '🔴', orange_circle: '🟠', yellow_circle: '🟡', green_circle: '🟢', large_blue_circle: '🔵',
  white_circle: '⚪', black_circle: '⚫', arrow_right: '➡️', arrow_left: '⬅️', arrow_up: '⬆️', arrow_down: '⬇️',
  repeat: '🔁', hundred: '💯', '100': '💯', new: '🆕', free: '🆓', soon: '🔜', top: '🔝', sos: '🆘',
  bug: '🐛', beetle: '🐞', construction: '🚧', rotating_light: '🚨', recycle: '♻️', seedling: '🌱',
  sunny: '☀️', cloud: '☁️', umbrella: '☔', snowflake: '❄️', rainbow: '🌈', earth_africa: '🌍', globe_with_meridians: '🌐',
  coffee: '☕', tea: '🍵', beer: '🍺', pizza: '🍕', cake: '🍰', apple: '🍎', house: '🏠', office: '🏢',
  car: '🚗', airplane: '✈️', train: '🚆', ship: '🚢', money_with_wings: '💸', moneybag: '💰', credit_card: '💳',
  handshake: '🤝', busts_in_silhouette: '👥', bust_in_silhouette: '👤', man_technologist: '👨‍💻',
  woman_technologist: '👩‍💻', robot: '🤖', ghost: '👻', skull: '💀', poop: '💩', see_no_evil: '🙈', dog: '🐶',
  cat: '🐱', unicorn: '🦄', turtle: '🐢', snail: '🐌',
});

/** A shortcode's emoji, or nothing for a code the table does not hold. */
export function emojiOf(shortcode: string): string | undefined {
  return Object.hasOwn(EMOJI_SHORTCODES, shortcode) ? EMOJI_SHORTCODES[shortcode] : undefined;
}

/** The `:code:` the text ends in, where the table holds it: where it starts and what replaces it. */
export function shortcodeAt(text: string): { readonly index: number; readonly text: string; readonly emoji: string } | null {
  const match = /(?:^|[\s(])(:([a-z0-9_+-]{1,40}):)$/.exec(text);
  if (!match) return null;
  const typed = match[1] as string;
  const emoji = emojiOf(match[2] as string);
  if (!emoji) return null;
  return { index: text.length - typed.length, text: typed, emoji };
}

/** Replaces a `:code:` the moment its closing colon is typed; undo brings the code back. */
export const EmojiShortcodes = Extension.create({
  name: 'emojiShortcodes',
  addInputRules() {
    return [new InputRule({
      find: (text: string) => {
        const found = shortcodeAt(text);
        return found ? { index: found.index, text: found.text, data: { emoji: found.emoji } } : null;
      },
      handler: ({ state, range, match }) => {
        const emoji = (match as { data?: { emoji?: string } }).data?.emoji;
        if (emoji) state.tr.insertText(emoji, range.from, range.to);
      },
    })];
  },
});
