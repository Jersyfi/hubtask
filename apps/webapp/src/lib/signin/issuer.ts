// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { IdentityProvider, IdentityProviderConfiguration, IdentityProviderPreset } from '@hubtask/sync-engine';

/**
 * The issuer a provider form holds once another provider is chosen.
 *
 * A provider with one issuer for every account has it filled in; what somebody typed stays; an
 * issuer the form filled for the provider chosen before is taken out again, because it belongs to
 * that provider and the save would refuse it as somebody else's. Called from the choice itself,
 * never from an effect: opening a stored provider sets the kind too, and its issuer must stay.
 */
export function issuerAfterChoosing(
  presets: readonly IdentityProviderPreset[],
  chosen: string,
  current: string,
): string {
  const typed = current.trim();
  const filledBefore = presets.some((each) => each.issuer && each.issuer === typed);
  if (typed !== '' && !filledBefore) return current;
  return presets.find((each) => each.kind === chosen)?.issuer ?? '';
}

/**
 * Whether the typed issuer is one of the preset's shared endpoints - its first path segment one of
 * `shared_issuer_paths` - which the server saves only with a directory list, in every mode.
 */
export function isSharedIssuer(preset: IdentityProviderPreset | undefined, issuer: string): boolean {
  return firstSegmentIn(preset?.shared_issuer_paths ?? [], issuer);
}

/**
 * Whether the typed issuer also serves personal accounts - its first path segment one of
 * `personal_issuer_paths` - where the server refuses `ANY` in every form.
 */
export function isPersonalIssuer(preset: IdentityProviderPreset | undefined, issuer: string): boolean {
  return firstSegmentIn(preset?.personal_issuer_paths ?? [], issuer);
}

/**
 * The admission modes a form offers: the ones the preset permits, without `ANY` on an issuer that
 * serves personal accounts. `ANY` stays while it is the current value - a row stored before the
 * refusal - so the form never shows a mode the row does not have; the form says why it cannot be
 * saved so, and the person chooses another.
 */
export function offeredModes<Mode extends string>(
  modes: readonly Mode[],
  personal: boolean,
  current: string,
): Mode[] {
  return modes.filter((mode) => mode !== 'ANY' || !personal || current === 'ANY');
}

/**
 * The first entry of a drawn directory list that names one of the preset's personal directories
 * (`personal_directories`), where the mode creates accounts - anything but `INVITED_ONLY` - and the
 * server refuses it (`identity_provider.directory_personal`). Compared trimmed and without regard
 * to case, as the server compares it; answered as typed, trimmed, for the sentence. `undefined`
 * where nothing is refused. Asked only of a list the form draws: one it does not draw is sent empty.
 */
export function personalDirectoryListed(
  preset: IdentityProviderPreset | undefined,
  provisioning: string,
  directories: readonly string[],
): string | undefined {
  if (provisioning === 'INVITED_ONLY') return undefined;
  const personal = (preset?.personal_directories ?? []).map((each) => each.trim().toLowerCase());
  if (personal.length === 0) return undefined;
  return directories.map((each) => each.trim()).find((each) => each !== '' && personal.includes(each.toLowerCase()));
}

function firstSegmentIn(segments: readonly string[], issuer: string): boolean {
  if (segments.length === 0) return false;
  let path: string;
  try {
    path = new URL(issuer.trim()).pathname;
  } catch {
    return false;
  }
  const first = path.replace(/^\//, '').split('/')[0]?.toLowerCase() ?? '';
  return first !== '' && segments.some((segment) => segment.toLowerCase() === first);
}

/** Which admission list a provider form draws, and whether the server requires it. */
export interface AdmissionLists {
  /** The directory list (`allowed_directories`), in the words of the preset's directory claim. */
  readonly directories: boolean;
  /** The address domains (`allowed_email_domains`), for a preset that names no directory. */
  readonly domains: boolean;
  /** Whether the drawn list is required rather than a narrowing. */
  readonly required: boolean;
}

/**
 * The list the server will read for this mode and issuer, and only that one (ADR-0071 §2): under
 * `DOMAINS` the directories or, without a directory claim, the domains; under `INVITED_ONLY` the
 * directories as an optional narrowing; and on a shared endpoint the directories in every mode,
 * required - `ANY` included, where admission reads them on a shared endpoint only.
 */
export function admissionLists(
  preset: IdentityProviderPreset | undefined,
  provisioning: string,
  shared: boolean,
): AdmissionLists {
  const claim = Boolean(preset?.directory_claim);
  const directories = claim && (shared || provisioning === 'DOMAINS' || provisioning === 'INVITED_ONLY');
  const domains = !claim && provisioning === 'DOMAINS';
  return { directories, domains, required: shared || provisioning === 'DOMAINS' };
}

/**
 * What a provider form holds, the same at the workspace and the installation: one form, so a field
 * the two levels share reads and saves alike. What only one level asks - newcomers, admission at
 * the installation's level, the position - stays with the screen that asks it.
 */
export interface ProviderDraft {
  kind: string;
  issuer: string;
  clientId: string;
  /** Typed or empty: empty keeps the sealed one, which nothing can read back. */
  clientSecret: string;
  displayName: string;
  provisioning: string;
  /** One per line or separated by commas - whichever somebody pastes. */
  domains: string;
  directories: string;
}

/** An empty form, admitting as strictly as the level asks for a new provider. */
export function emptyDraft(provisioning: string): ProviderDraft {
  return { kind: 'GENERIC', issuer: '', clientId: '', clientSecret: '', displayName: '', provisioning, domains: '', directories: '' };
}

/** The form opened on a stored provider. The secret stays empty, because nothing can read it. */
export function draftOf(provider: IdentityProvider): ProviderDraft {
  return {
    kind: provider.kind,
    issuer: provider.issuer,
    clientId: provider.client_id,
    clientSecret: '',
    displayName: provider.display_name,
    provisioning: provider.provisioning,
    domains: (provider.allowed_email_domains ?? []).join('\n'),
    directories: (provider.allowed_directories ?? []).join('\n'),
  };
}

/** A list as written: one per line or separated by commas, blanks dropped. */
export function readList(written: string): string[] {
  return written
    .split(/[\n,]/)
    .map((one) => one.trim())
    .filter((one) => one !== '');
}

/** The personal directory the drawn list names where the save would refuse it, or nothing. */
export function draftRefusal(draft: ProviderDraft, preset: IdentityProviderPreset | undefined): string | undefined {
  const lists = admissionLists(preset, draft.provisioning, isSharedIssuer(preset, draft.issuer));
  return lists.directories ? personalDirectoryListed(preset, draft.provisioning, readList(draft.directories)) : undefined;
}

/**
 * The fields both levels send. Only the list drawn is sent: one typed under another mode and hidden
 * since is not saved unseen. An empty secret is left out, which keeps the sealed one - an empty
 * string would be a secret of no characters - and an empty name is left out for the issuer's host.
 */
export function draftBody(
  draft: ProviderDraft,
  preset: IdentityProviderPreset | undefined,
): Omit<IdentityProviderConfiguration, 'position'> {
  const lists = admissionLists(preset, draft.provisioning, isSharedIssuer(preset, draft.issuer));
  return {
    issuer: draft.issuer.trim(),
    client_id: draft.clientId.trim(),
    ...(draft.clientSecret === '' ? {} : { client_secret: draft.clientSecret }),
    ...(draft.displayName.trim() === '' ? {} : { display_name: draft.displayName.trim() }),
    kind: draft.kind as IdentityProvider['kind'],
    provisioning: draft.provisioning as IdentityProvider['provisioning'],
    allowed_email_domains: lists.domains ? readList(draft.domains) : [],
    allowed_directories: lists.directories ? readList(draft.directories) : [],
  };
}
