// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The workspaces an installed app holds, as its screens use them (UC-SYN-07 check 4, UC-ID-25
 * check 7): *Switch workspace*, *Add a workspace* and *Forget this workspace*. Where they are kept
 * is the platform's (`platform/appSession.ts`); what is asked of a server - ending the app's
 * session there - is `engine.ts`'s, and what the open window lets go of is the session's.
 */

import { groupedByInstallation, type AppWorkspace } from '../platform/appSession.ts';
import { platform } from '../platform/index.ts';
import { session } from '../session.svelte.ts';
import { endAppSession } from './engine.ts';
import { queue } from './queue.svelte.ts';

/** A row of *Switch workspace*: the workspace, and whether the keystore holds a session for it. */
export interface AppWorkspaceRow extends AppWorkspace {
  readonly isSignedIn: boolean;
}

const revoke = (held: { readonly server: string; readonly access: string; readonly refresh: string }) =>
  endAppSession(held.server, held);

class AppWorkspaceList {
  #rows = $state<readonly AppWorkspaceRow[]>([]);

  /** Whether this build holds workspaces of its own: the installed apps only. */
  get isHeld(): boolean {
    return platform.appWorkspaces !== undefined;
  }

  get current(): AppWorkspace | undefined {
    return platform.appWorkspaces?.current;
  }

  /** The rows, grouped under each installation's host (the approved preview's choice). */
  get groups(): readonly { readonly installation: string; readonly workspaces: readonly AppWorkspaceRow[] }[] {
    return groupedByInstallation(this.#rows) as readonly { installation: string; workspaces: readonly AppWorkspaceRow[] }[];
  }

  /** The workspace the list holds at a host, if any: *Add a workspace* refuses a second one. */
  heldAt(host: string): AppWorkspace | undefined {
    return platform.appWorkspaces?.list().find((each) => each.host === host);
  }

  /** Reads the list and asks the keystore which of them hold a session. */
  async show(): Promise<void> {
    const held = platform.appWorkspaces;
    if (held === undefined) return;
    const list = held.list();
    const signedIn = await Promise.all(list.map((each) => held.isSignedIn(each.server)));
    this.#rows = list.map((each, index) => ({ ...each, isSignedIn: signedIn[index] === true }));
  }

  /** *Open*: the window starts on that workspace, from its own copy, with no browser involved. */
  open(server: string): void {
    platform.appWorkspaces?.open(server);
  }

  /** How many changes made offline would go with it: the queue here, the copy's elsewhere. */
  async waiting(server: string): Promise<number> {
    if (server === this.current?.server && session.isSignedIn) return queue.count;
    return (await platform.appWorkspaces?.waiting(server)) ?? 0;
  }

  /** *Forget this workspace*: its grant, session, copy and key; the window moves on where it was open. */
  async forget(server: string): Promise<void> {
    await platform.appWorkspaces?.forget(server, revoke, () => session.endHere());
    await this.show();
  }

  /** At start: the grants a forgetting while offline still owes, withdrawn where they answer now. */
  settle(): void {
    void platform.appWorkspaces?.settle(revoke).catch(() => {});
  }
}

export const appWorkspaces = new AppWorkspaceList();
