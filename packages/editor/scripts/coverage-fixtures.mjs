// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Writes the updates and snapshots the server's reader of update coverage is tested against
// (core/application/service/realtime, Coverage_test.go): real editors' updates, from seeded random
// editing by three writers who exchange them out of order, each case with the snapshot of a writer
// that applied all of them and of one that missed one.
//
//   node scripts/coverage-fixtures.mjs > ../../core/application/service/realtime/testdata/coverage.json

import * as Y from 'yjs';

let seed = 0x5eed;
const random = () => {
  seed = (seed * 1103515245 + 12345) & 0x7fffffff;
  return seed / 0x80000000;
};
const pick = (list) => list[Math.floor(random() * list.length)];
const int = (n) => Math.floor(random() * n);
const hex = (bytes) => Buffer.from(bytes).toString('hex');

const words = ['a', 'Wort', 'größer', '😀 emoji', '日本語', '', 'x'.repeat(40), '\u{1F600}\u{1F601}'];
const values = () => pick([
  null, true, false, int(1000), -int(100000), random() * 1e6, 2 ** 60, pick(words),
  { nested: { list: [1, 'two', { three: 3 }] } }, [pick(words), int(9)], new Uint8Array([1, 2, 3]),
]);

function edit(doc) {
  const text = doc.getText('text');
  const map = doc.getMap('map');
  const array = doc.getArray('array');
  const xml = doc.getXmlFragment('prosemirror');
  switch (int(9)) {
    case 0: text.insert(int(text.length + 1), pick(words)); break;
    case 1: if (text.length > 0) { const at = int(text.length); text.delete(at, 1 + int(Math.min(5, text.length - at))); } break;
    case 2: if (text.length > 0) text.format(0, 1 + int(text.length), { bold: random() < 0.5 ? true : null }); break;
    case 3: map.set(pick(['k1', 'k2', 'k3']), values()); break;
    case 4: map.delete(pick(['k1', 'k2', 'k3'])); break;
    case 5: array.insert(int(array.length + 1), [values(), values()]); break;
    case 6: if (array.length > 0) array.delete(int(array.length), 1); break;
    case 7: {
      const element = new Y.XmlElement(pick(['paragraph', 'heading']));
      element.setAttribute('level', String(int(3)));
      const inner = new Y.XmlText();
      inner.insert(0, pick(words));
      inner.insertEmbed(0, { image: 'x' });
      element.insert(0, [inner]);
      xml.insert(int(xml.length + 1), [element]);
      break;
    }
    case 8: {
      if (xml.length > 0 && random() < 0.6) xml.delete(int(xml.length), 1);
      else map.set('sub', new Y.Doc({ guid: `sub-${int(100)}` }));
      break;
    }
  }
}

function run(gc) {
  const writers = [1, 2, 3].map((id) => {
    const doc = new Y.Doc({ gc });
    doc.clientID = 1000 + id * 7919 + int(1 << 20);
    return doc;
  });
  const log = [];
  for (const doc of writers) {
    doc.on('update', (update, origin) => {
      if (origin !== 'remote') log.push(update);
    });
  }
  const delivered = writers.map(() => 0);
  for (let step = 0; step < 60; step++) {
    const doc = pick(writers);
    edit(doc);
    // Each writer catches up to a random place in the log, so its edits build on others'.
    const i = writers.indexOf(doc);
    const upTo = delivered[i] + int(log.length - delivered[i] + 1);
    for (const update of log.slice(delivered[i], upTo)) Y.applyUpdate(doc, update, 'remote');
    delivered[i] = upTo;
  }
  const shuffled = [...log].sort(() => random() - 0.5);
  const whole = new Y.Doc({ gc });
  for (const update of shuffled) Y.applyUpdate(whole, update, 'remote');
  // The update missed must carry content, or missing it is no loss.
  const missed = log.findIndex((u, i) => i > log.length / 2 && Y.decodeUpdate(u).structs.length > 0);
  const partial = new Y.Doc({ gc });
  for (const update of shuffled) if (update !== log[missed]) Y.applyUpdate(partial, update, 'remote');
  const merged = Y.mergeUpdates(log.slice(0, Math.ceil(log.length / 2)));
  return {
    gc,
    updates: log.map(hex),
    ends: log.map((u) => Object.fromEntries(Y.parseUpdateMeta(u).to)),
    merged: hex(merged),
    snapshot: hex(Y.encodeStateAsUpdate(whole)),
    partial: hex(Y.encodeStateAsUpdate(partial)),
  };
}

const cases = [];
for (let i = 0; i < 12; i++) cases.push(run(i % 3 !== 0));
process.stdout.write(`${JSON.stringify({ cases }, null, 1)}\n`);
