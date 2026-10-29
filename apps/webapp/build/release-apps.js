// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The installed apps in a release (UC-SYN-07 check 1, F7 Decisions 9 and 10): what `release.yml`
// decides about them, kept here so a test can hold it rather than a run on a tag. The version each
// build is stamped with (the product version plus ADR-0035's platform build counter), the names
// the files are published under, the update manifest, the check of every update signature and of
// the APK's certificate against the keys versioning-release.md §8 records, and the one checksum
// file over everything the release publishes. Node's own modules only: the release job runs it
// before anything is installed.

import crypto from 'node:crypto';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

/** The checksum file beside the release's other files, in the form `sha256sum --check` reads. */
export const CHECKSUMS = 'SHA256SUMS';
/** The update manifest the desktop updater and the Android notice read (ADR-0097). */
export const MANIFEST = 'latest.json';
/**
 * The desktop updater's endpoint: the manifest as a download with its signature, in the updater's
 * single-download format, so the updater checks the manifest itself under the update key before
 * it reads a package from it (`src-tauri/src/update.rs`).
 */
export const POINTER = 'update.json';
/** Where the shell asks for it: the newest release that is no pre-release. */
export const POINTER_ENDPOINT = `https://github.com/Jersyfi/hubtask/releases/latest/download/${POINTER}`;
/** Where Windows finds SQLCipher's libcrypto: beside the executable, so the installer carries it. */
export const WINDOWS_CRYPTO_DLL = 'libcrypto-3-x64.dll';

// The fourth part of a Windows file version, where the counter goes, holds at most this much.
const MAX_COUNTER = 65535;
const TAG = /^v(\d+\.\d+\.\d+)(-[0-9A-Za-z.-]+)?$/;

/** The product version a tag names, `v0.9.0-rc.1` → `0.9.0-rc.1`; anything else throws. */
export function productVersion(tag) {
  const match = TAG.exec(tag);
  if (!match) throw new Error(`${tag} is not a release tag vX.Y.Z or vX.Y.Z-pre`);
  return `${match[1]}${match[2] ?? ''}`;
}

/**
 * The version a build of the apps carries: the product version with the platform build counter
 * as SemVer build metadata, which orders nothing, so an app never takes its own release for a newer
 * one. The desktop bundler writes the counter into the Windows file version and the package version.
 */
export function stampedVersion(tag, counter) {
  const count = Number(counter);
  if (!Number.isInteger(count) || count < 1 || count > MAX_COUNTER) {
    throw new Error(`the build counter ${counter} is not a whole number from 1 to ${MAX_COUNTER}`);
  }
  return `${productVersion(tag)}+${count}`;
}

/**
 * The configuration `tauri build --config` merges over `tauri.conf.json` for one target: the
 * stamped version everywhere, the counter as Android's `versionCode`, and on Windows the libcrypto
 * the shell links (`dll`, relative to the shell's directory) with the C runtime it needs.
 */
export function buildConfig(target, tag, counter, dll) {
  const version = stampedVersion(tag, counter);
  switch (target) {
    case 'linux':
      return { version };
    case 'windows':
      if (!dll) throw new Error('the Windows build needs the path of libcrypto');
      return { version, bundle: { resources: { [dll]: WINDOWS_CRYPTO_DLL }, windows: { bundleVCRuntime: true } } };
    case 'android':
      return { version, bundle: { android: { versionCode: Number(counter) } } };
    default:
      throw new Error(`${target} is not one of linux, windows, android`);
  }
}

/** The four files a release offers, by what the bundler names them and what they are published as. */
const KINDS = [
  { kind: 'windows', match: /-setup\.exe$/, name: (v) => `hubtask-${v}-windows-x64-setup.exe`, platforms: ['windows-x86_64'] },
  { kind: 'appimage', match: /\.AppImage$/, name: (v) => `hubtask-${v}-linux-x86_64.AppImage`, platforms: ['linux-x86_64', 'linux-x86_64-appimage'] },
  { kind: 'deb', match: /\.deb$/, name: (v) => `hubtask-${v}-linux-amd64.deb`, platforms: ['linux-x86_64-deb'] },
  { kind: 'apk', match: /\.apk$/, name: (v) => `hubtask-${v}-android-universal.apk`, platforms: ['android'] },
];

/** The published names of a release's apps, one per kind, for the product version of `tag`. */
export function publishedNames(tag) {
  const version = productVersion(tag);
  return Object.fromEntries(KINDS.map(({ kind, name }) => [kind, name(version)]));
}

/**
 * Which built file becomes which published one: every kind found exactly once among `files`
 * (paths the bundlers wrote), or the list of what is missing or doubled.
 */
export function assignNames(files, tag) {
  const names = publishedNames(tag);
  const problems = [];
  const plan = [];
  for (const { kind, match } of KINDS) {
    const found = files.filter((file) => match.test(file));
    if (found.length !== 1) problems.push(`${kind}: ${found.length === 0 ? 'no file was built' : `${found.length} files, ${found.join(', ')}`}`);
    else plan.push({ from: found[0], to: names[kind] });
  }
  if (problems.length > 0) throw new Error(`the release's apps are incomplete - ${problems.join('; ')}`);
  return plan;
}

/**
 * The update manifest: the stamped version, the release's page as its notes, and per platform the
 * download and its signature (the content of the `.sig` file beside it in `dir`). Every app is in
 * it, the APK too: the Android app reads the same manifest to name a newer release (ADR-0097).
 */
export function updateManifest({ dir, tag, counter, base, date }) {
  const names = publishedNames(tag);
  const platforms = {};
  for (const { kind, platforms: keys } of KINDS) {
    const file = names[kind];
    const signature = fs.readFileSync(path.join(dir, `${file}.sig`), 'utf8').trim();
    for (const key of keys) platforms[key] = { signature, url: `${base}/${file}` };
  }
  return { version: stampedVersion(tag, counter), notes: `${base.replace(/\/download\/[^/]+$/, '/tag/')}${tag}`, pub_date: new Date(date).toISOString(), platforms };
}

/** The updater's endpoint for the release: the signed manifest as its one download. */
export function updatePointer({ dir, tag, counter, base }) {
  const signature = fs.readFileSync(path.join(dir, `${MANIFEST}.sig`), 'utf8').trim();
  return { version: stampedVersion(tag, counter), url: `${base}/${MANIFEST}`, signature };
}

/**
 * What in the shell's configuration (`tauri.conf.json`) does not trust the release: the updater
 * must name the key §8 records - the public key file the signer writes, base64 as Tauri reads it -
 * and ask the pointer. A placeholder never reaches a release.
 */
export function shellKeyProblems(config, updateKey) {
  const updater = config?.plugins?.updater ?? {};
  const problems = [];
  const file = Buffer.from(String(updater.pubkey ?? ''), 'base64').toString('utf8').split('\n');
  if (file[1]?.trim() !== updateKey) problems.push("tauri.conf.json's updater does not name the update key versioning-release.md §8 records");
  if (JSON.stringify(updater.endpoints) !== JSON.stringify([POINTER_ENDPOINT])) problems.push(`tauri.conf.json's updater does not ask ${POINTER_ENDPOINT}`);
  if (updater.requireSignedVersion !== true) problems.push("tauri.conf.json's updater does not require the signed version");
  return problems;
}

/** The keys versioning-release.md §8 records, read from its table; `pending` until the owner fills it. */
export function recordedKeys(document) {
  const cell = (label) => {
    const row = document.split('\n').find((line) => line.startsWith(`| ${label} |`));
    const value = row?.match(/`([^`]+)`/)?.[1];
    if (!value || value === 'pending') throw new Error(`versioning-release.md §8 records no ${label} yet - the owner creates the key first`);
    return value;
  };
  const apk = cell('APK signing certificate, SHA-256').replaceAll(':', '').toLowerCase();
  if (!/^[0-9a-f]{64}$/.test(apk)) throw new Error('the recorded APK certificate digest is not a SHA-256');
  return { apkCertificate: apk, updateKey: cell('Update public key') };
}

/** A public key of the updater's signature format (minisign): its 8-byte id and the Ed25519 key. */
function parsePublicKey(line) {
  const raw = Buffer.from(line.trim(), 'base64');
  if (raw.length !== 42 || raw.subarray(0, 2).toString() !== 'Ed') throw new Error('the update key is not an Ed25519 public key of the updater');
  const key = crypto.createPublicKey({ key: { kty: 'OKP', crv: 'Ed25519', x: raw.subarray(10).toString('base64url') }, format: 'jwk' });
  return { id: raw.subarray(2, 10), key };
}

/** The update key's id as the signer prints it, which versioning-release.md names beside it. */
export function keyId(line) {
  return Buffer.from(parsePublicKey(line).id).reverse().toString('hex').toUpperCase();
}

/**
 * Whether `signature` (a `.sig` file's content, base64 of the signature text) signs `bytes` under
 * `publicKey` - the file's signature and the trusted comment's, which binds the version to it.
 */
export function signatureHolds(bytes, signature, publicKey) {
  const { id, key } = parsePublicKey(publicKey);
  const lines = Buffer.from(signature.trim(), 'base64').toString('utf8').split('\n');
  if (lines.length < 4 || !lines[2].startsWith('trusted comment: ')) return false;
  const sig = Buffer.from(lines[1], 'base64');
  if (sig.length !== 74 || !sig.subarray(2, 10).equals(id)) return false;
  const algorithm = sig.subarray(0, 2).toString();
  if (algorithm !== 'Ed' && algorithm !== 'ED') return false;
  const message = algorithm === 'ED' ? crypto.createHash('blake2b512').update(bytes).digest() : bytes;
  const fileSignature = sig.subarray(10);
  if (!crypto.verify(null, message, key, fileSignature)) return false;
  const comment = Buffer.from(lines[2].slice('trusted comment: '.length), 'utf8');
  return crypto.verify(null, Buffer.concat([fileSignature, comment]), key, Buffer.from(lines[3], 'base64'));
}

/**
 * What in `dir` is not signed under `publicKey`: each published app and the manifest need a `.sig`,
 * and the pointer carries the manifest's.
 */
export function signatureProblems(dir, tag, publicKey) {
  const problems = [];
  const pointer = path.join(dir, POINTER);
  const manifestSignature = path.join(dir, `${MANIFEST}.sig`);
  if (!fs.existsSync(pointer)) problems.push(`${POINTER} is missing`);
  else if (fs.existsSync(manifestSignature) && JSON.parse(fs.readFileSync(pointer, 'utf8')).signature !== fs.readFileSync(manifestSignature, 'utf8').trim()) {
    problems.push(`${POINTER} does not carry ${MANIFEST}'s signature`);
  }
  for (const file of [...Object.values(publishedNames(tag)), MANIFEST]) {
    const sig = path.join(dir, `${file}.sig`);
    if (!fs.existsSync(sig)) problems.push(`${file} has no signature`);
    else if (!signatureHolds(fs.readFileSync(path.join(dir, file)), fs.readFileSync(sig, 'utf8'), publicKey)) {
      problems.push(`${file}'s signature does not hold under the recorded update key`);
    }
  }
  return problems;
}

function sha256(file) {
  return crypto.createHash('sha256').update(fs.readFileSync(file)).digest('hex');
}

/** The checksum file's content over every file in `dir` but itself, sorted by name. */
export function checksums(dir) {
  return fs
    .readdirSync(dir, { withFileTypes: true })
    .filter((entry) => entry.isFile() && entry.name !== CHECKSUMS)
    .map((entry) => entry.name)
    .sort()
    .map((name) => `${sha256(path.join(dir, name))}  ${name}\n`)
    .join('');
}

/** What does not match between `dir` and its checksum file: a file changed, missing or unlisted. */
export function checksumProblems(dir) {
  const listed = new Map();
  for (const line of fs.readFileSync(path.join(dir, CHECKSUMS), 'utf8').split('\n').filter(Boolean)) {
    const match = /^([0-9a-f]{64}) {2}(.+)$/.exec(line);
    if (!match) return [`${CHECKSUMS} has a line that is not "<sha256>  <name>": ${line}`];
    listed.set(match[2], match[1]);
  }
  const problems = [];
  const present = fs.readdirSync(dir).filter((name) => name !== CHECKSUMS);
  for (const name of present) if (!listed.has(name)) problems.push(`${name} is not in ${CHECKSUMS}`);
  for (const [name, digest] of listed) {
    const file = path.join(dir, name);
    if (!fs.existsSync(file)) problems.push(`${name} is listed but missing`);
    else if (sha256(file) !== digest) problems.push(`${name} does not match its checksum`);
  }
  return problems;
}

function walk(dir) {
  return fs.readdirSync(dir, { withFileTypes: true }).flatMap((entry) => {
    const full = path.join(dir, entry.name);
    return entry.isDirectory() ? walk(full) : [full];
  });
}

function fail(problems) {
  for (const problem of problems) console.error(`release-apps: ${problem}`);
  process.exit(1);
}

const usage = `usage: release-apps.js
  version <tag> <counter>                                     the version every build carries
  config <linux|windows|android> <tag> <counter> [libcrypto]  the --config of one build
  collect <from-dir> <to-dir> <tag>                           the built apps under their published names
  key <apk|update> <versioning-release.md>                    a key §8 records
  manifest <dir> <tag> <counter> <download-base> <date>       write ${MANIFEST}
  pointer <dir> <tag> <counter> <download-base>               write ${POINTER}, once ${MANIFEST} is signed
  verify-signatures <dir> <tag> <versioning-release.md> <tauri.conf.json>
                                                              every app and the manifest signed, the shell trusting the key
  checksums <dir>                                             write ${CHECKSUMS}
  check <dir>                                                 the files match ${CHECKSUMS}`;

function main([command, ...args]) {
  try {
    switch (command) {
      case 'version':
        console.log(stampedVersion(args[0], args[1]));
        return;
      case 'config':
        console.log(JSON.stringify(buildConfig(args[0], args[1], args[2], args[3])));
        return;
      case 'collect': {
        const [from, to, tag] = args;
        fs.mkdirSync(to, { recursive: true });
        for (const { from: file, to: name } of assignNames(walk(from), tag)) {
          fs.copyFileSync(file, path.join(to, name));
          console.log(`${path.relative(from, file)} -> ${name}`);
        }
        return;
      }
      case 'key': {
        const keys = recordedKeys(fs.readFileSync(args[1], 'utf8'));
        console.log(args[0] === 'apk' ? keys.apkCertificate : keys.updateKey);
        return;
      }
      case 'manifest': {
        const [dir, tag, counter, base, date] = args;
        fs.writeFileSync(path.join(dir, MANIFEST), `${JSON.stringify(updateManifest({ dir, tag, counter, base, date }), null, 2)}\n`);
        return;
      }
      case 'pointer': {
        const [dir, tag, counter, base] = args;
        fs.writeFileSync(path.join(dir, POINTER), `${JSON.stringify(updatePointer({ dir, tag, counter, base }), null, 2)}\n`);
        return;
      }
      case 'verify-signatures': {
        const [dir, tag, document, shellConfig] = args;
        const { updateKey } = recordedKeys(fs.readFileSync(document, 'utf8'));
        const problems = [
          ...signatureProblems(dir, tag, updateKey),
          ...shellKeyProblems(JSON.parse(fs.readFileSync(shellConfig, 'utf8')), updateKey),
        ];
        if (problems.length > 0) fail(problems);
        return;
      }
      case 'checksums':
        fs.writeFileSync(path.join(args[0], CHECKSUMS), checksums(args[0]));
        return;
      case 'check': {
        const problems = checksumProblems(args[0]);
        if (problems.length > 0) fail(problems);
        return;
      }
      default:
        console.error(usage);
        process.exit(2);
    }
  } catch (error) {
    fail([error instanceof Error ? error.message : String(error)]);
  }
}

if (process.argv[1] === fileURLToPath(import.meta.url)) main(process.argv.slice(2));
