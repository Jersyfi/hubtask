// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * Facilitation on a board (research §4.2): a voting session and a timer every participant sees.
 * Both live in the board's live document beside its objects - the `facilitation` map - so they
 * reach everyone drawing at once and stay out of the scene the board saves.
 *
 * **Votes.** A session gives each person a number of votes to spread over the objects, several on
 * one if they like. A person's votes are kept under a voter key: their account in an open session,
 * a key their device minted in an anonymous one, so the document never names who voted what.
 *
 * **The timer** keeps when it ends (or, paused, what was left), not a ticking number, so every
 * device counts down by its own clock.
 */

import * as Y from 'yjs';

export interface Voting {
  id: string;
  /** Votes each person may give. */
  perPerson: number;
  anonymous: boolean;
  open: boolean;
  startedBy: string;
}

export interface Timer {
  /** Milliseconds since the epoch the timer ends at, while it runs. */
  endsAt?: number;
  /** Milliseconds left, while it is paused. */
  left?: number;
  /** What it was started with, in milliseconds. */
  length: number;
}

export const MAX_VOTES_PER_PERSON = 20;
export const MAX_TIMER = 4 * 60 * 60 * 1000;

/** A board's voting session and timer in its live document. */
export class Facilitation {
  readonly #map: Y.Map<unknown>;
  readonly #votes: Y.Map<string[]>;

  constructor(doc: Y.Doc) {
    this.#map = doc.getMap('facilitation');
    this.#votes = doc.getMap<string[]>('votes');
  }

  get voting(): Voting | undefined {
    const raw = this.#map.get('voting') as Partial<Voting> | undefined;
    if (!raw || typeof raw.id !== 'string') return undefined;
    return { id: raw.id, perPerson: Number(raw.perPerson ?? 3), anonymous: raw.anonymous === true, open: raw.open === true, startedBy: String(raw.startedBy ?? '') };
  }

  /** Starts a new session; the votes of an earlier one are cleared. */
  startVoting(session: Omit<Voting, 'open'>): void {
    const perPerson = Math.max(1, Math.min(MAX_VOTES_PER_PERSON, Math.round(session.perPerson)));
    this.#map.doc?.transact(() => {
      this.#votes.clear();
      this.#map.set('voting', { ...session, perPerson, open: true });
    });
  }

  /** Ends the session; its results stay until the next one starts or they are cleared. */
  endVoting(): void {
    const voting = this.voting;
    if (voting) this.#map.set('voting', { ...voting, open: false });
  }

  clearVoting(): void {
    this.#map.doc?.transact(() => {
      this.#votes.clear();
      this.#map.delete('voting');
    });
  }

  /** The objects a voter gave their votes to, one entry per vote. */
  votesOf(voter: string): string[] {
    const voting = this.voting;
    return voting ? [...(this.#votes.get(`${voting.id}:${voter}`) ?? [])] : [];
  }

  /** Gives one vote to an object; refused once the voter has none left or the session is closed. */
  vote(voter: string, elementId: string): boolean {
    const voting = this.voting;
    if (!voting?.open) return false;
    const mine = this.votesOf(voter);
    if (mine.length >= voting.perPerson) return false;
    this.#votes.set(`${voting.id}:${voter}`, [...mine, elementId]);
    return true;
  }

  /** Takes one of the voter's votes back from an object. */
  unvote(voter: string, elementId: string): boolean {
    const voting = this.voting;
    if (!voting?.open) return false;
    const mine = this.votesOf(voter);
    const at = mine.indexOf(elementId);
    if (at < 0) return false;
    mine.splice(at, 1);
    this.#votes.set(`${voting.id}:${voter}`, mine);
    return true;
  }

  /** The votes each object got, most first; and how many people voted. */
  results(): { counts: Map<string, number>; voters: number } {
    const voting = this.voting;
    const counts = new Map<string, number>();
    let voters = 0;
    if (!voting) return { counts, voters };
    this.#votes.forEach((ids, key) => {
      if (!key.startsWith(`${voting.id}:`) || ids.length === 0) return;
      voters += 1;
      for (const id of ids.slice(0, voting.perPerson)) counts.set(id, (counts.get(id) ?? 0) + 1);
    });
    return { counts: new Map([...counts].sort((a, b) => b[1] - a[1])), voters };
  }

  get timer(): Timer | undefined {
    const raw = this.#map.get('timer') as Partial<Timer> | undefined;
    if (!raw || typeof raw.length !== 'number') return undefined;
    return { length: raw.length, ...(typeof raw.endsAt === 'number' ? { endsAt: raw.endsAt } : {}), ...(typeof raw.left === 'number' ? { left: raw.left } : {}) };
  }

  startTimer(length: number, now: number): void {
    const bounded = Math.max(1000, Math.min(MAX_TIMER, Math.round(length)));
    this.#map.set('timer', { length: bounded, endsAt: now + bounded });
  }

  pauseTimer(now: number): void {
    const timer = this.timer;
    if (timer?.endsAt !== undefined) this.#map.set('timer', { length: timer.length, left: Math.max(0, timer.endsAt - now) });
  }

  resumeTimer(now: number): void {
    const timer = this.timer;
    if (timer?.left !== undefined) this.#map.set('timer', { length: timer.length, endsAt: now + timer.left });
  }

  stopTimer(): void {
    this.#map.delete('timer');
  }

  /** What the timer shows at an instant: milliseconds left, and whether it runs. */
  remaining(now: number): { left: number; running: boolean } | undefined {
    const timer = this.timer;
    if (!timer) return undefined;
    if (timer.endsAt !== undefined) return { left: Math.max(0, timer.endsAt - now), running: timer.endsAt > now };
    return { left: timer.left ?? 0, running: false };
  }

  observe(changed: () => void): () => void {
    const handler = () => changed();
    this.#map.observe(handler);
    this.#votes.observe(handler);
    return () => {
      this.#map.unobserve(handler);
      this.#votes.unobserve(handler);
    };
  }
}
