// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';

import { dueInputFor } from '../data/due.ts';
import { dateIn, instantOf, timeIn } from '../i18n/zone.ts';
import { NOTHING, dueOf, literalKey, readLine, readTargets, type Context, type Recognition } from './parse.ts';
import { vocabularyOf, type Vocabulary } from './words.ts';

const catalogue = (language: string): Record<string, string> =>
  JSON.parse(readFileSync(new URL(`../../../../../locales/${language}.json`, import.meta.url), 'utf8')) as Record<string, string>;
const EN = catalogue('en');
const DE = catalogue('de');

/** The catalogue chain as the client renders it: the reader's file, then the source. */
const vocabulary = (language: 'en' | 'de', drop: readonly string[] = []): Vocabulary =>
  vocabularyOf((code) => (language === 'de' && !drop.includes(code) ? (DE[code] ?? EN[code]) : EN[code]) ?? '');

const HUBS = [
  { id: 'hub-house', name: 'House' },
  { id: 'hub-client', name: 'Client A' },
];
const COLLECTIONS = [
  { id: 'kitchen', name: 'Kitchen', hubId: 'hub-house' },
  { id: 'kitchen-garden', name: 'Kitchen garden', hubId: 'hub-house' },
  { id: 'client-kitchen', name: 'Kitchen', hubId: 'hub-client' },
  // Composed here; typed decomposed below.
  { id: 'kueche', name: 'Küche', hubId: 'hub-house' },
];
const PEOPLE = [
  { id: 'jerome', name: 'Jérôme Winkel' },
  { id: 'mara', name: 'Mara Lind' },
];
const LABELS = [
  { id: 'urgent', name: 'Urgent' },
  { id: 'materials', name: 'Materials' },
];
const TASKS: Recognition = { date: true, person: true, label: true, people: PEOPLE, labels: LABELS };

/** The instant the clock in `zone` shows this day and time, plus some seconds. */
const at = (date: string, time: string, zone: string, seconds = 0) => Date.parse(instantOf(date, time, zone)!) + seconds * 1000;

function context(overrides: Partial<Context> = {}): Context {
  return {
    now: at('2026-10-21', '12:00', 'Europe/Berlin'),
    zone: 'Europe/Berlin',
    words: vocabulary('en'),
    hubs: HUBS,
    collections: COLLECTIONS,
    currentHubId: 'hub-house',
    literal: new Set(),
    ...overrides,
  };
}

function read(text: string, overrides: Partial<Context> = {}, recognition: Recognition = TASKS) {
  const ctx = context(overrides);
  const reading = readLine(text, ctx, readTargets(text, ctx), recognition);
  return { ...reading, due: dueOf(reading.chips, ctx), kinds: reading.chips.map((chip) => `${chip.kind}:${chip.text}`) };
}

// ------------------------------------------------------------------------------------------------
// Chips and the title
// ------------------------------------------------------------------------------------------------

test('the story line: a day, a time, a person and a label, and the rest is the title', () => {
  const reading = read('Call the electrician tomorrow 11am +Mara !Urgent');
  assert.equal(reading.title, 'Call the electrician');
  assert.deepEqual(reading.kinds, ['date:tomorrow', 'time:11am', 'person:+Mara', 'label:!Urgent']);
  assert.deepEqual(reading.due, { date: '2026-10-22', time: '11:00' });
  assert.equal(reading.chips.find((chip) => chip.kind === 'person')?.id, 'mara');
  assert.equal(reading.chips.find((chip) => chip.kind === 'label')?.id, 'urgent');
});

test('nothing recognised: the line is the title', () => {
  const reading = read('Water the plants');
  assert.equal(reading.title, 'Water the plants');
  assert.deepEqual(reading.chips, []);
});

test('a removed chip keeps its text in the title', () => {
  const literal = new Set([literalKey('date', 'tomorrow'), literalKey('label', '!Urgent')]);
  const reading = read('Call tomorrow !Urgent', { literal });
  assert.equal(reading.title, 'Call tomorrow !Urgent');
  assert.deepEqual(reading.chips, []);
});

test('names with spaces need no quotes, and the longest wins', () => {
  assert.deepEqual(read('Invoice #Client A now').kinds, ['hub:#Client A']);
  const garden = readTargets('Plant @Kitchen garden beans', context());
  assert.equal(garden.collection?.id, 'kitchen-garden');
  assert.equal(readTargets('Paint @Kitchen walls', context()).collection?.id, 'kitchen');
});

test('a name must end where a word ends', () => {
  assert.equal(readTargets('Ask @Kitchenette', context()).collection, undefined);
  assert.deepEqual(read('Ask +Marabou').kinds, []);
});

test('a name typed decomposed matches its composed twin', () => {
  const typed = 'Abwasch @Küche';
  assert.notEqual(typed, 'Abwasch @Küche');
  assert.equal(readTargets(typed, context()).collection?.id, 'kueche');
});

test('two collections of one name: the named hub, else the reader’s hub, else the first', () => {
  assert.equal(readTargets('Tiles @Kitchen #Client A', context()).collection?.id, 'client-kitchen');
  assert.equal(readTargets('Tiles @Kitchen', context({ currentHubId: 'hub-client' })).collection?.id, 'client-kitchen');
  assert.equal(readTargets('Tiles @Kitchen', context({ currentHubId: undefined })).collection?.id, 'kitchen');
});

test('a sigil inside a word is text', () => {
  assert.deepEqual(read('mail me@Kitchen and 1+Mara').kinds, []);
});

test('a first name answers where nobody shares it', () => {
  assert.equal(read('Call +Mara').chips[0]?.id, 'mara');
  assert.equal(read('Call +Mara Lind').chips[0]?.text, '+Mara Lind');
});

test('a person the target does not offer is text: a contributor names only themselves', () => {
  const onlySelf: Recognition = { ...TASKS, people: [PEOPLE[0]!] };
  assert.deepEqual(read('Call +Mara', {}, onlySelf).kinds, []);
  assert.deepEqual(read('Call +Jérôme', {}, onlySelf).kinds, ['person:+Jérôme']);
});

test('an unknown label is text; capture makes no label', () => {
  assert.deepEqual(read('Fix !Urgnt').kinds, []);
});

test('several labels, each once', () => {
  assert.deepEqual(read('Order !Materials !Urgent !Materials').kinds, ['label:!Materials', 'label:!Urgent']);
});

// ------------------------------------------------------------------------------------------------
// What the target recognises
// ------------------------------------------------------------------------------------------------

test('a target that recognises nothing - the jumble, a List - leaves the words in the title', () => {
  const reading = read('Milk tomorrow +Mara !Urgent #House', {}, NOTHING);
  assert.equal(reading.title, 'Milk tomorrow +Mara !Urgent');
  assert.deepEqual(reading.kinds, ['hub:#House']);
});

test('a collection without dates takes no date, and keeps the person and labels it carries', () => {
  const reading = read('Call tomorrow +Mara', {}, { ...TASKS, date: false });
  assert.equal(reading.title, 'Call tomorrow');
  assert.deepEqual(reading.kinds, ['person:+Mara']);
});

// ------------------------------------------------------------------------------------------------
// Narrow on purpose
// ------------------------------------------------------------------------------------------------

test('a bare number is a count, not a time', () => {
  assert.deepEqual(read('Buy 11 eggs').kinds, []);
  assert.deepEqual(read('Buy 1.5 kg flour').kinds, []);
});

test('a time needs a colon, am or pm, or the hour word', () => {
  assert.deepEqual(read('Call 14:30').due?.time, '14:30');
  assert.deepEqual(read('Call 2pm').due?.time, '14:00');
  assert.deepEqual(read('Call 2 pm').due?.time, '14:00');
  assert.deepEqual(read('Call 12am').due?.time, '00:00');
  assert.deepEqual(read('Call 11 a.m.').due?.time, '11:00');
  assert.deepEqual(read("Call 9 o'clock").due?.time, '09:00');
  assert.deepEqual(read('Call 25:00').kinds, []);
  assert.deepEqual(read('Call 13pm').kinds, []);
});

test('a month name needs a day beside it', () => {
  assert.deepEqual(read('Ask whether we may go').kinds, []);
  assert.deepEqual(read('March in the parade').kinds, []);
  assert.deepEqual(read('Meet 3 May').due, { date: '2027-05-03' });
  assert.deepEqual(read('Meet May 3rd').due, { date: '2027-05-03' });
  assert.deepEqual(read('Meet Dec 24').due, { date: '2026-12-24' });
});

test('an abbreviated weekday needs its full stop or a time after it', () => {
  // 2026-10-21 is a Wednesday.
  assert.deepEqual(read('Sun and sea').kinds, []);
  assert.deepEqual(read('Wed the two of them').kinds, []);
  assert.deepEqual(read('Pay rent Fri.').due, { date: '2026-10-23' });
  assert.deepEqual(read('Pay rent Fri 10:00').due, { date: '2026-10-23', time: '10:00' });
  assert.deepEqual(read('Pay rent Friday').due, { date: '2026-10-23' });
});

test('a weekday said on that weekday is next week’s', () => {
  assert.deepEqual(read('Report Wednesday').due, { date: '2026-10-28' });
});

test('relative days, numeric days and ISO days', () => {
  assert.deepEqual(read('Call today').due, { date: '2026-10-21' });
  assert.deepEqual(read('Call the day after tomorrow').due, { date: '2026-10-23' });
  assert.deepEqual(read('Call in 3 days').due, { date: '2026-10-24' });
  assert.deepEqual(read('Call 2026-11-03').due, { date: '2026-11-03' });
  assert.deepEqual(read('Call 11/3').due, { date: '2026-11-03' });
  assert.deepEqual(read('Call 10/1').due, { date: '2027-10-01' });
  assert.deepEqual(read('Call 2026-02-30').kinds, []);
});

test('a time alone is today while it is ahead, else tomorrow', () => {
  assert.deepEqual(read('Call 11am').due, { date: '2026-10-22', time: '11:00' });
  assert.deepEqual(read('Call 3pm').due, { date: '2026-10-21', time: '15:00' });
});

// ------------------------------------------------------------------------------------------------
// Midnight, in several zones (check 5)
// ------------------------------------------------------------------------------------------------

const ZONES = ['Pacific/Kiritimati', 'Pacific/Pago_Pago', 'America/St_Johns', 'Asia/Kolkata', 'Europe/Berlin'];

for (const zone of ZONES) {
  test(`"tomorrow" near midnight is the day after the person's today in ${zone}`, () => {
    const before = read('Call tomorrow', { zone, now: at('2026-10-21', '23:59', zone, 30) });
    assert.deepEqual(before.due, { date: '2026-10-22' });
    const after = read('Call tomorrow', { zone, now: at('2026-10-22', '00:00', zone, 30) });
    assert.deepEqual(after.due, { date: '2026-10-23' });

    // The stored shape: an all-day date in the person's zone, read back as that day there.
    const wire = dueInputFor(after.due!, zone)!;
    assert.equal(wire.due_date_only, true);
    assert.equal(wire.due_time_zone, zone);
    assert.equal(dateIn(wire.due_at, zone), '2026-10-23');
  });
}

test('the hour that happens twice: Berlin on the night the clocks go back', () => {
  const zone = 'Europe/Berlin';
  const reading = read('Check the heating tomorrow 02:30', { zone, now: at('2026-10-24', '12:00', zone) });
  assert.deepEqual(reading.due, { date: '2026-10-25', time: '02:30' });
  const wire = dueInputFor(reading.due!, zone)!;
  assert.equal(wire.due_date_only, false);
  assert.equal(dateIn(wire.due_at, zone), '2026-10-25');
  assert.equal(timeIn(wire.due_at, zone), '02:30');

  const today = read('Check today 02:30', { zone, now: at('2026-10-25', '00:10', zone) });
  assert.deepEqual(today.due, { date: '2026-10-25', time: '02:30' });
});

test('a midnight that does not exist: Santiago on the morning its clocks spring forward', () => {
  const zone = 'America/Santiago';
  const reading = read('Water the garden tomorrow', { zone, now: at('2026-09-05', '23:00', zone) });
  assert.deepEqual(reading.due, { date: '2026-09-06' });
  const wire = dueInputFor(reading.due!, zone)!;
  assert.equal(dateIn(wire.due_at, zone), '2026-09-06');
});

// ------------------------------------------------------------------------------------------------
// The person's language
// ------------------------------------------------------------------------------------------------

test('German: relative days, the hour word, weekdays and dates as German writes them', () => {
  const german = { words: vocabulary('de') };
  assert.deepEqual(read('Elektriker anrufen morgen 11 Uhr', german).due, { date: '2026-10-22', time: '11:00' });
  assert.equal(read('Elektriker anrufen morgen 11 Uhr', german).title, 'Elektriker anrufen');
  assert.deepEqual(read('Miete übermorgen', german).due, { date: '2026-10-23' });
  assert.deepEqual(read('Miete Freitag', german).due, { date: '2026-10-23' });
  assert.deepEqual(read('Miete 3.11.', german).due, { date: '2026-11-03' });
  assert.deepEqual(read('Miete 3. Mai', german).due, { date: '2027-05-03' });
  assert.deepEqual(read('Miete in 3 Tagen', german).due, { date: '2026-10-24' });
  // German writes no am or pm: "3 am Montag" is "3 on Monday", never three in the morning.
  assert.deepEqual(read('Ruf 3 am Montag an', german).kinds, ['date:am Montag']);
  assert.equal(read('Ruf 3 am Montag an', german).title, 'Ruf 3 an');
  // An English word in a German line is text.
  assert.deepEqual(read('Miete tomorrow', german).kinds, []);
});

test('the words and the names come from one language: a catalogue without the words is English throughout', () => {
  const missing = Object.keys(EN).filter((code) => code.startsWith('app.capture.words.'));
  const fallen = vocabulary('de', missing);
  assert.equal(fallen.language, 'en');
  assert.ok(fallen.weekdays.includes('friday'));
  assert.ok(!fallen.weekdays.includes('freitag'));
});

test('a bare hour after a day is its time where the line ends or a token follows, a count before a word', () => {
  const german = { words: vocabulary('de') };
  assert.deepEqual(read('Startseite texten morgen 10 @Kitchen', german).due, { date: '2026-10-22', time: '10:00' });
  assert.deepEqual(read('Startseite texten morgen 10', german).kinds, ['date:morgen', 'time:10']);
  assert.equal(read('Kaufe morgen 10 Eier', german).title, 'Kaufe 10 Eier');
  assert.equal(read('Buy tomorrow 10 eggs').title, 'Buy 10 eggs');
});

test('the word before a day or a time is the day\'s, not the title\'s', () => {
  const german = { words: vocabulary('de') };
  assert.equal(read('Anruf am 30.10.', german).title, 'Anruf');
  assert.deepEqual(read('Anruf am Freitag um 9 Uhr', german).kinds, ['date:am Freitag', 'time:um 9 Uhr']);
  assert.equal(read('Meet on Friday at 10:00').title, 'Meet');
});

test('every word code the source has, the German catalogue has too', () => {
  for (const code of Object.keys(EN).filter((each) => each.startsWith('app.capture.words.'))) {
    assert.ok(code in DE, code);
  }
});
