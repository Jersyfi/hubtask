// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Storage } from '@hubtask/sync-engine';

import type { Area } from '../router.ts';

import type { AppWorkspace, Ending, HeldSession } from './appSession.ts';
import type { SessionPair } from './tokenStore.ts';
import type { AppUpdate } from './update.ts';
import type { DeviceWorkspaces } from './workspaces.ts';

/**
 * The platform seam of ADR-0033: everything conditioned on where this bundle runs lives behind
 * this interface, so no `isTauri` conditional ever lands in a component. The implementation is
 * chosen at build time, never by runtime sniffing: the import of `browser.ts` at the end of this
 * file is redirected to the target's own by `vite.config.ts` (`target.ts`), so the browser bundle
 * carries no shell's module and a shell's carries no browser's.
 *
 * The interface is deliberately as small as what exists: a port is added here with its first real
 * implementation, not before.
 */
export interface Platform {
  /**
   * Which target this bundle serves: `browser`, or the Tauri shells' `desktop` and `mobile`. A capability difference between targets is expressed as a field on this interface,
   * never as a check on this value scattered through components.
   */
  readonly target: 'browser' | 'desktop' | 'mobile';

  /**
   * Where the API answers: the page's own origin in the browser, a relative `/api/v1` (ADR-0028);
   * the server the person chose in a shell, which is served from inside itself (ADR-0031).
   */
  readonly apiBase: string;

  /**
   * Where the workspace's web app answers, for an address somebody else opens - a copied or sent
   * link to an entry: the page's own origin in the browser, the server's in a shell, whose own
   * origin (`tauri://localhost`) means nothing outside it.
   */
  readonly webOrigin: string;

  /**
   * The bearer for an API call, or `undefined` when nobody is signed in.
   *
   * A function rather than a value, and here rather than in the sync engine, because *where* a
   * token is kept is exactly what differs between targets: the browser holds it for the tab's
   * lifetime, and the shells hold it in the platform keystore (ADR-0031). The engine asks per call
   * so that a refresh, or a sign-out, is seen by the next request rather than by the next reload.
   */
  bearer(): string | undefined;

  /**
   * The credential the exchange at `/auth/sessions:refresh` presents, or `undefined`.
   *
   * Beside `bearer` rather than inside it, because the two are used by different callers at
   * different moments: every request reads the bearer, and exactly one caller - the engine's
   * refresher - ever reads this one, when a request has been refused.
   */
  refreshToken(): string | undefined;

  /**
   * Holds the pair a sign-in answered for the calls that follow.
   *
   * `POST /auth/sessions` answers an access token of fifteen minutes and a refresh token of
   * thirty days, and every route takes the first as its bearer. *Where* the pair is kept is
   * exactly what differs between targets - the browser holds it for the tab's lifetime, and the
   * shells hold it in the platform keystore (ADR-0031) - which is why it is here rather than in
   * the sync engine.
   */
  holdSession(pair: SessionPair): void;

  /**
   * Holds the next pair of the same session, as a refresh answered it.
   *
   * Apart from `holdSession` because what belongs to the session - its elevation to the control
   * plane - stays, where a new session's pair drops it (ADR-0070 §4, UC-INS-04 check 3).
   */
  renewSession(pair: SessionPair): void;

  /**
   * Forgets it. What sign-out calls, and what a refused exchange calls before the sign-in screen.
   *
   * The credentials only; everything else the client holds is dropped by the caller
   * (`offline-sync.md` §9.6), because this seam knows about a pair and not about a cache.
   */
  releaseBearer(): void;

  /**
   * Follows a download target the server minted.
   *
   * Here rather than in a component for the reason the bearer is: this is exactly what differs
   * between targets. A browser navigates to the URL and lets `Content-Disposition: attachment` do
   * the rest; a shell hands the URL to the platform's own downloader, which is what puts a file in
   * the folder a person expects rather than in a sandbox (ADR-0031).
   *
   * The URL is always one the server answered — a presigned bucket URL or this server's content
   * route — never one composed here, and it expires.
   */
  openDownload(url: string): void;

  /**
   * Puts a file the client already holds in front of the person.
   *
   * The export is a `POST` — a view's query is the caller's content and a query string travels
   * through access logs — so its answer arrives as bytes rather than as a place to navigate to,
   * and `openDownload` cannot deliver it. A browser makes an object URL and clicks it; a shell
   * writes to the folder a person picked, which is exactly the difference this seam exists for
   * (ADR-0031).
   */
  saveFile(bytes: Blob, fileName: string): void;

  /**
   * The languages this reader's device says they read, best first.
   *
   * Here rather than read from `navigator` in a view, for the reason every platform difference is:
   * a shell asks its own operating system. It orders a widened search — a hit in a language
   * somebody reads is a hit they can act on — and never decides whether one happens.
   */
  preferredLanguages(): readonly string[];

  /**
   * The replica's store for one account, or nothing where this runtime has none to offer
   * (ADR-0033 §4).
   *
   * One store per API origin and account, opened here because the platform is what knows where a
   * copy may live: the browser's IndexedDB - a best-effort cache, never the offline promise, and
   * with no encryption of its own by decision - or a shell's SQLite under the platform keystore.
   * Nothing under `apps/` touches IndexedDB itself; the package holds the store, this seam names it.
   */
  storageFor(accountId: string): Storage | undefined;

  /**
   * Whether that store may hold a file captured offline until it can be uploaded (offline-sync.md
   * §1): the shells' encrypted copy may, the browser's unencrypted one may not - there a capture
   * without the server is refused, `sync.needs_connection`. What the engine's `holdsFiles` is set to.
   */
  readonly holdsFiles: boolean;

  /**
   * How an empty copy is first filled (the engine's `firstSynchronisation`): the browser reads the
   * snapshot in one response; the installed apps walk the pages and hold each one's cursor, so a
   * first synchronisation the network or the system cut short resumes where it stopped (SY-B).
   */
  readonly firstSynchronisation: 'snapshot' | 'pages';

  /**
   * What sign-out removes beyond the store the engine deletes: the shells' key for this account's
   * copy, so that nothing left on the device can read it (offline-sync.md §9.6). The browser's
   * store has no key, and answers at once.
   */
  forgetCopy(accountId: string): Promise<void>;

  /**
   * Which account the held pair belongs to, remembered once `/accounts/me` has said and gone with
   * the pair. What a tab that reloads while the server cannot be reached opens its replica by.
   */
  rememberAccount(id: string): void;
  lastAccount(): string | undefined;

  /**
   * When this session's elevation to the control plane ends, and where it is remembered
   * (ADR-0070 §4).
   *
   * Beside the pair, and for the reason the account is: a reload must not lose it. **Losing it is
   * worse than keeping it** — the door would ask for a second proof while the first hour still
   * stands at the server, and passing it would start a *new* hour, which is the one thing "the
   * elevation does not slide" forbids.
   *
   * A moment, not a credential: a client that wrote itself a later one is refused by the server on
   * its next call, because the scope is granted per request from the session's own row.
   */
  rememberElevation(until: string): void;
  elevatedUntil(): string | undefined;

  /**
   * How this device introduces itself in the workspace's device list - "Firefox on Linux" in the
   * browser, the name the system gives the machine in a shell.
   */
  deviceName(): string;

  /** The platform beside that name: `web` in the browser, the system's own name in a shell. */
  readonly devicePlatform: string;

  /**
   * The device's list of workspaces (UC-ID-25, ADR-0095 §7): host and name of each workspace this
   * device signed in to, as the installation signed them. Here because where the list lives is
   * exactly what differs: the browser keeps it at the installation's own address and moves through
   * it by redirects; a shell keeps it without a switch and offers `appWorkspaces` instead.
   */
  readonly workspaces: DeviceWorkspaces;

  /**
   * The installed apps' sign-in through the system browser (ADR-0096), absent in the browser,
   * whose sign-in card is the page itself. A passkey's relying party is the workspace's host,
   * which a webview's origin never is, so the app opens the workspace's own card in the system's
   * browser and takes the code back at a loopback address.
   */
  readonly browserSignIn?: BrowserSignIn;

  /**
   * The workspaces an installed app holds (UC-SYN-07 check 4, UC-ID-25 check 7), absent in the
   * browser, whose list is the installation's (`workspaces`). The app holds any number, of one
   * installation or several, each with its own session in the keystore and its own copy.
   */
  readonly appWorkspaces?: AppWorkspaces;

  /**
   * The areas this build leaves to the web app (ADR-0032, UC-SYN-07 check 7), absent where every
   * area is here: the Android app's account menu names them with the web app's address and opens
   * it in the system's browser, and an address into one shows the page that says so.
   */
  readonly webApp?: WebAppDoor;

  /**
   * Hands an entry's title and link to the system's share sheet (UC-SYN-07 check 8), absent where
   * there is none to hand it to: the browser and the desktop offer *Copy link* alone.
   */
  readonly sendLink?: (title: string, url: string) => Promise<void>;

  /**
   * Paints the system's bars in the page's background, with icons that read on it: the Android
   * app's status and navigation bars. Absent where the page has no bars of the system's to paint.
   */
  readonly paintBars?: (background: string, isDark: boolean) => void;

  /**
   * Staying current (UC-SYN-07 checks 5 and 6, ADR-0097), absent in the browser, which the server
   * keeps current: the desktop's updater, which installs once the person agrees, and the Android
   * app's notice, which opens the release's download.
   */
  readonly updates?: AppUpdates;
}

/** What the installed apps know about a newer release. */
export interface AppUpdates {
  /** This app's product version. */
  readonly current: string;
  /**
   * Where the offer stands: on every page of the desktop app, on *Today* in the Android app; and
   * how long *Later* hides it - until the next start, or until a newer version than the one hidden.
   */
  readonly offersOn: 'every-page' | 'today';
  readonly laterLasts: 'start' | 'version';
  /** Looks for a newer release; nothing when there is none, or none can be read now. */
  check(): Promise<AppUpdate | undefined>;
  /** Installs what `check` found ready and starts the new version. */
  install(): Promise<void>;
  /** Opens a page of the releases in the system's browser. */
  open(url: string): Promise<void>;
}

/** The web app, for what this build leaves out. */
export interface WebAppDoor {
  readonly areas: readonly Area[];
  /** Opens an address of the web app in the system's browser. */
  open(url: string): Promise<void>;
}

/** What the installed apps' account menu and *Switch workspace* act on. */
export interface AppWorkspaces {
  /** The workspace this window is open on, signed in or not; nothing on first start. */
  readonly current: AppWorkspace | undefined;
  /** Every workspace the app holds, in the order they were added. */
  list(): readonly AppWorkspace[];
  /** Whether the keystore holds a session for it. */
  isSignedIn(server: string): Promise<boolean>;
  /** How many changes made offline wait in its copy. */
  waiting(server: string): Promise<number>;
  /** Starts the window on another workspace, from its own session and copy. */
  open(server: string): void;
  /**
   * Withdraws its grant and deletes its session, copy and key (`forgetWorkspace`); the window
   * starts on the next one where it was open. `revoke` ends a session at its server.
   */
  forget(server: string, revoke: (held: HeldSession) => Promise<Ending>, stopHere: () => Promise<void>): Promise<void>;
  /** Withdraws the grants a forgetting while offline still owes, where their server answers now. */
  settle(revoke: (held: HeldSession) => Promise<Ending>): Promise<void>;
}

/** What the redirect brought back to the loopback address. */
export interface ReturnedCode {
  readonly code?: string;
  readonly state?: string;
  readonly error?: string;
}

/**
 * The loopback page: the words in the reader's language, and the card's look - the resolved tokens
 * as a stylesheet and the mark as drawn (`loopbackPage.ts`) - since the page can load nothing.
 */
export interface LoopbackPage {
  readonly title: string;
  readonly body: string;
  readonly lang: string;
  readonly style?: string;
  readonly mark?: string;
}

/** What only the shell can do for the sign-in through the browser. */
export interface BrowserSignIn {
  /** The workspace the app was last signed in to, kept through a sign-out; nothing on first start. */
  readonly workspace: AppWorkspace | undefined;
  /** This app's version, which a server must reach (F7 Decision 12). */
  readonly appVersion: string;
  /** The system the built-in client names: `windows`, `linux` or `android`. */
  readonly appPlatform: string;
  /**
   * Opens the loopback listener, whose page says `page` once the code is taken, and answers its
   * port; `returned` hears the redirect once. A second call retires the first listener.
   */
  listen(page: LoopbackPage, returned: (answer: ReturnedCode) => void): Promise<number>;
  /** Stops waiting: *Use another address*. */
  retire(): void;
  /**
   * Waits for the sign-in's code and answers the redirect the consent sends it to: the loopback
   * listener's on the desktop, the app's own `eu.hubtask.app:/oauth` on Android (identity.md
   * §15.3), which the system hands back to the app. `returned` hears it once.
   */
  listenForSignIn(page: LoopbackPage, returned: (answer: ReturnedCode) => void): Promise<string>;
  /** Hands an address of the workspace to the system's browser. */
  openInBrowser(url: string): Promise<void>;
  /** Holds the new session in the keystore and starts the window on it. */
  enter(workspace: AppWorkspace, pair: SessionPair): Promise<void>;
  /** Remembers the workspace's name once the app has read it. */
  nameWorkspace(name: string): void;
  /** Whether this run began with a sign-in, taken once. */
  takeArrival(): boolean;
}

export type { SessionPair } from './tokenStore.ts';
export type { AppUpdate } from './update.ts';
export type { AppWorkspace, Ending, HeldSession } from './appSession.ts';
export type { DeviceWorkspaces, Hop, KnownWorkspace } from './workspaces.ts';
export { platform } from './browser.ts';
