// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * Facilitation on a board (research §4.2): a voting session and a timer every participant sees.
 * Both live in the board's live document beside its objects - the `facilitation` map - so they
 * reach everyone drawing at once and stay out of the scene the board saves.
 *
 * **Votes.** A session gives each person a number of votes to spread over the objects, several on
 * one if they like. A person's votes are kept under a voter key: their account in an open session,
 * a key their device minted in an anonymous one, so the document never names who voted what. Each
 * vote is a key of its own (`session:voter#vote`), so two devices of one voter voting at once
 * keep both; the votes past the voter's number, given at once, are not counted. A board that kept
 * a voter's votes as one list (`session:voter`) is still read.
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

/** One vote: the key it is kept under, the object, and its place in a voter's list if listed. */
interface Ballot {
  key: string;
  elementId: string;
  listed?: number;
}

/** A board's voting session and timer in its live document. */
export class Facilitation {
  readonly #map: Y.Map<unknown>;
  readonly #votes: Y.Map<string[] | string>;

  constructor(doc: Y.Doc) {
    this.#map = doc.getMap('facilitation');
    this.#votes = doc.getMap<string[] | string>('votes');
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
    return voting ? (this.#ballots(voting).get(voter) ?? []).map((b) => b.elementId) : [];
  }

  /** Gives one vote to an object; refused once the voter has none left or the session is closed. */
  vote(voter: string, elementId: string): boolean {
    const voting = this.voting;
    if (!voting?.open) return false;
    if (this.votesOf(voter).length >= voting.perPerson) return false;
    this.#votes.set(`${voting.id}:${voter}#${globalThis.crypto.randomUUID()}`, elementId);
    return true;
  }

  /** Takes one of the voter's votes back from an object. */
  unvote(voter: string, elementId: string): boolean {
    const voting = this.voting;
    if (!voting?.open) return false;
    const ballot = [...(this.#ballots(voting).get(voter) ?? [])].reverse().find((b) => b.elementId === elementId);
    if (!ballot) return false;
    if (ballot.listed === undefined) {
      this.#votes.delete(ballot.key);
      return true;
    }
    const kept = [...((this.#votes.get(ballot.key) as string[] | undefined) ?? [])];
    kept.splice(ballot.listed, 1);
    this.#votes.set(ballot.key, kept);
    return true;
  }

  /** The votes each object got, most first; and how many people voted. */
  results(): { counts: Map<string, number>; voters: number } {
    const voting = this.voting;
    const counts = new Map<string, number>();
    let voters = 0;
    if (!voting) return { counts, voters };
    for (const ballots of this.#ballots(voting).values()) {
      if (ballots.length === 0) continue;
      voters += 1;
      for (const { elementId } of ballots) counts.set(elementId, (counts.get(elementId) ?? 0) + 1);
    }
    return { counts: new Map([...counts].sort((a, b) => b[1] - a[1])), voters };
  }

  /**
   * The session's votes by voter, in one order on every device - the listed ones first, then by
   * key - and at most the voter's number: votes given at once past it are not counted.
   */
  #ballots(voting: Voting): Map<string, Ballot[]> {
    const prefix = `${voting.id}:`;
    const byVoter = new Map<string, Ballot[]>();
    const add = (voter: string, ballot: Ballot) => {
      const list = byVoter.get(voter) ?? [];
      list.push(ballot);
      byVoter.set(voter, list);
    };
    const keys = [...this.#votes.keys()].filter((key) => key.startsWith(prefix)).sort();
    for (const key of keys) {
      const value = this.#votes.get(key);
      const rest = key.slice(prefix.length);
      const mark = rest.indexOf('#');
      if (Array.isArray(value) && mark < 0) {
        value.forEach((elementId, listed) => add(rest, { key, elementId, listed }));
      } else if (typeof value === 'string' && mark > 0) {
        add(rest.slice(0, mark), { key, elementId: value });
      }
    }
    for (const [voter, list] of byVoter) {
      byVoter.set(voter, [...list.filter((b) => b.listed !== undefined), ...list.filter((b) => b.listed === undefined)].slice(0, voting.perPerson));
    }
    return byVoter;
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
