// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import assert from 'node:assert/strict';
import crypto from 'node:crypto';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { test } from 'node:test';
import { fileURLToPath } from 'node:url';

import {
  CHECKSUMS,
  MANIFEST,
  POINTER,
  POINTER_ENDPOINT,
  assignNames,
  buildConfig,
  checksumProblems,
  checksums,
  keyId,
  publishedNames,
  recordedKeys,
  shellKeyProblems,
  signatureHolds,
  signatureProblems,
  stampedVersion,
  updateManifest,
  updatePointer,
} from './release-apps.js';

const repository = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..', '..', '..');

// A throwaway key of the Tauri CLI's signer and its signature of the five bytes `hello`, written by
// `tauri signer sign --app-version 0.9.0+42`: the verifier reads what the release's signer writes.
const SIGNER_KEY = 'RWRD4T7szKI8KtF3cIVk95WX5mMrBvuXOfJf10OtDOkg6HQwEDh9EXO/';
const SIGNER_SIGNATURE =
  'dW50cnVzdGVkIGNvbW1lbnQ6IHNpZ25hdHVyZSBmcm9tIHRhdXJpIHNlY3JldCBrZXkKUlVSRDRUN3N6S0k4S2lsNWFBQWtNQjFlMXd0Ni85dGRWbGhJNHdOMDFmUlMrcXQ0Y2ZFeFhVS253bVBSUi9Cc0hhUlNOS1U0eXlFSzU0Ym5QcFUwUklpSitzNnpyaGZzOUFZPQp0cnVzdGVkIGNvbW1lbnQ6IHRpbWVzdGFtcDoxNzkzMjkwMjg4CWZpbGU6YS5iaW4JdmVyc2lvbjowLjkuMCs0MgpXcXRjL1QyK1NUcXNwTEZRMCtGeGllQm1wV2R5aFV5SkVkbmNMNjAxZmZkaTVVYUFaNnlRZzEvOVVUSVN3d2crZDVEaTllbFVZRkppNkFtbDdsWVlBdz09Cg==';

/** A key of the updater's format made here, and a signer that writes `.sig` content as the CLI does. */
function updateKey() {
  const { publicKey, privateKey } = crypto.generateKeyPairSync('ed25519');
  const id = crypto.randomBytes(8);
  const raw = Buffer.from(publicKey.export({ format: 'jwk' }).x, 'base64url');
  const line = Buffer.concat([Buffer.from('Ed'), id, raw]).toString('base64');
  const sign = (bytes, comment = 'timestamp:0\tfile:x') => {
    const fileSignature = crypto.sign(null, crypto.createHash('blake2b512').update(bytes).digest(), privateKey);
    const global = crypto.sign(null, Buffer.concat([fileSignature, Buffer.from(comment)]), privateKey);
    const text = [
      'untrusted comment: signature from tauri secret key',
      Buffer.concat([Buffer.from('ED'), id, fileSignature]).toString('base64'),
      `trusted comment: ${comment}`,
      global.toString('base64'),
      '',
    ].join('\n');
    return Buffer.from(text).toString('base64');
  };
  return { line, sign };
}

function tempDir() {
  return fs.mkdtempSync(path.join(os.tmpdir(), 'hubtask-release-'));
}

const DOCUMENT = (apk, update) =>
  `## 8. The apps' keys\n\n| Key | Recorded |\n|---|---|\n| APK signing certificate, SHA-256 | \`${apk}\` |\n| Update public key | \`${update}\` |\n`;

test('a build carries the product version and the platform build counter', () => {
  assert.equal(stampedVersion('v0.9.0', 42), '0.9.0+42');
  assert.equal(stampedVersion('v1.0.0-rc.2', '7'), '1.0.0-rc.2+7');
  for (const tag of ['0.9.0', 'v0.9', 'v0.9.0+3', 'release-1']) assert.throws(() => stampedVersion(tag, 1), /not a release tag/);
  for (const counter of [0, -1, 1.5, 'x', 65536]) assert.throws(() => stampedVersion('v0.9.0', counter), /build counter/);
});

test('each target is built with its own configuration', () => {
  assert.deepEqual(buildConfig('linux', 'v0.9.0', 42), { version: '0.9.0+42' });
  assert.deepEqual(buildConfig('android', 'v0.9.0', 42), { version: '0.9.0+42', bundle: { android: { versionCode: 42 } } });
  const windows = buildConfig('windows', 'v0.9.0', 42, 'target/windows-dlls/libcrypto-3-x64.dll');
  assert.deepEqual(windows.bundle.resources, { 'target/windows-dlls/libcrypto-3-x64.dll': 'libcrypto-3-x64.dll' });
  assert.equal(windows.bundle.windows.bundleVCRuntime, true);
  assert.throws(() => buildConfig('windows', 'v0.9.0', 42), /libcrypto/);
  assert.throws(() => buildConfig('macos', 'v0.9.0', 42), /not one of/);
});

test('every built app is published once under its name, and a missing or doubled one stops the release', () => {
  const built = [
    'shells/windows/nsis/Hubtask_0.9.0+42_x64-setup.exe',
    'shells/linux/appimage/Hubtask_0.9.0+42_amd64.AppImage',
    'shells/linux/deb/Hubtask_0.9.0+42_amd64.deb',
    'shells/android/universal/release/app-universal-release-unsigned.apk',
  ];
  assert.deepEqual(
    assignNames(built, 'v0.9.0').map(({ to }) => to),
    ['hubtask-0.9.0-windows-x64-setup.exe', 'hubtask-0.9.0-linux-x86_64.AppImage', 'hubtask-0.9.0-linux-amd64.deb', 'hubtask-0.9.0-android-universal.apk'],
  );
  assert.throws(() => assignNames(built.slice(1), 'v0.9.0'), /windows: no file was built/);
  assert.throws(() => assignNames([...built, 'other/x.deb'], 'v0.9.0'), /deb: 2 files/);
});

test('the update key reads as the signer writes it, and its signature holds only over its file', () => {
  assert.equal(keyId(SIGNER_KEY), '2A3CA2CCEC3EE143');
  assert.equal(signatureHolds(Buffer.from('hello'), SIGNER_SIGNATURE, SIGNER_KEY), true);
  assert.equal(signatureHolds(Buffer.from('hellO'), SIGNER_SIGNATURE, SIGNER_KEY), false);
  assert.equal(signatureHolds(Buffer.from('hello'), SIGNER_SIGNATURE, updateKey().line), false);
  // The trusted comment carries the version; one changed after signing breaks the signature.
  const text = Buffer.from(SIGNER_SIGNATURE, 'base64').toString('utf8').replace('version:0.9.0+42', 'version:9.9.9+42');
  assert.equal(signatureHolds(Buffer.from('hello'), Buffer.from(text).toString('base64'), SIGNER_KEY), false);
});

test('the keys come from versioning-release.md, and a key not yet recorded stops the release', () => {
  const digest = 'AB:'.repeat(31) + 'AB';
  assert.deepEqual(recordedKeys(DOCUMENT(digest, SIGNER_KEY)), { apkCertificate: 'ab'.repeat(32), updateKey: SIGNER_KEY });
  assert.throws(() => recordedKeys(DOCUMENT('pending', SIGNER_KEY)), /records no APK signing certificate/);
  assert.throws(() => recordedKeys(DOCUMENT(digest, 'pending')), /records no Update public key/);
  assert.throws(() => recordedKeys(DOCUMENT('abc', SIGNER_KEY)), /not a SHA-256/);
  // The real document holds the table the release reads, whatever it records today.
  const document = fs.readFileSync(path.join(repository, 'docs', 'architecture', 'versioning-release.md'), 'utf8');
  assert.ok(/^\| APK signing certificate, SHA-256 \| `[^`]+` \|$/m.test(document), 'versioning-release.md §8 has no APK certificate row');
  assert.ok(/^\| Update public key \| `[^`]+` \|$/m.test(document), 'versioning-release.md §8 has no update key row');
});

/** A release directory as the job assembles it: the apps signed, the manifest written and signed. */
function release(key) {
  const dir = tempDir();
  for (const name of Object.values(publishedNames('v0.9.0'))) {
    fs.writeFileSync(path.join(dir, name), `the bytes of ${name}`);
    fs.writeFileSync(path.join(dir, `${name}.sig`), key.sign(fs.readFileSync(path.join(dir, name))));
  }
  const base = 'https://github.com/owner/hubtask/releases/download/v0.9.0';
  const manifest = updateManifest({ dir, tag: 'v0.9.0', counter: 42, base, date: '2026-10-29T12:00:00+01:00' });
  fs.writeFileSync(path.join(dir, MANIFEST), JSON.stringify(manifest));
  fs.writeFileSync(path.join(dir, `${MANIFEST}.sig`), key.sign(fs.readFileSync(path.join(dir, MANIFEST))));
  fs.writeFileSync(path.join(dir, POINTER), JSON.stringify(updatePointer({ dir, tag: 'v0.9.0', counter: 42, base })));
  for (const other of ['sbom.cdx.json', 'hubtask-0.9.0.tgz', 'THIRD-PARTY-LICENSES.md']) fs.writeFileSync(path.join(dir, other), other);
  return { dir, manifest };
}

test('the manifest names every app with its download and signature', () => {
  const key = updateKey();
  const { dir, manifest } = release(key);
  assert.equal(manifest.version, '0.9.0+42');
  assert.equal(manifest.notes, 'https://github.com/owner/hubtask/releases/tag/v0.9.0');
  assert.equal(manifest.pub_date, '2026-10-29T11:00:00.000Z');
  assert.deepEqual(Object.keys(manifest.platforms).sort(), ['android', 'linux-x86_64', 'linux-x86_64-appimage', 'linux-x86_64-deb', 'windows-x86_64']);
  const windows = manifest.platforms['windows-x86_64'];
  assert.equal(windows.url, 'https://github.com/owner/hubtask/releases/download/v0.9.0/hubtask-0.9.0-windows-x64-setup.exe');
  assert.equal(windows.signature, fs.readFileSync(path.join(dir, 'hubtask-0.9.0-windows-x64-setup.exe.sig'), 'utf8'));
  assert.deepEqual(signatureProblems(dir, 'v0.9.0', key.line), []);
});

test('an app or a manifest that is unsigned, or signed by another key, is named', () => {
  const key = updateKey();
  const { dir } = release(key);
  fs.rmSync(path.join(dir, 'hubtask-0.9.0-linux-amd64.deb.sig'));
  fs.writeFileSync(path.join(dir, `${MANIFEST}.sig`), updateKey().sign(fs.readFileSync(path.join(dir, MANIFEST))));
  assert.deepEqual(signatureProblems(dir, 'v0.9.0', key.line), [
    "update.json does not carry latest.json's signature",
    'hubtask-0.9.0-linux-amd64.deb has no signature',
    "latest.json's signature does not hold under the recorded update key",
  ]);
});

test('the pointer hands the updater the manifest with its signature, and one that does not is named', () => {
  const key = updateKey();
  const { dir } = release(key);
  const pointer = JSON.parse(fs.readFileSync(path.join(dir, POINTER), 'utf8'));
  assert.equal(pointer.version, '0.9.0+42');
  assert.equal(pointer.url, 'https://github.com/owner/hubtask/releases/download/v0.9.0/latest.json');
  assert.equal(pointer.signature, fs.readFileSync(path.join(dir, `${MANIFEST}.sig`), 'utf8').trim());
  fs.writeFileSync(path.join(dir, POINTER), JSON.stringify({ ...pointer, signature: key.sign(Buffer.from('another manifest')) }));
  assert.deepEqual(signatureProblems(dir, 'v0.9.0', key.line), ["update.json does not carry latest.json's signature"]);
  fs.rmSync(path.join(dir, POINTER));
  assert.deepEqual(signatureProblems(dir, 'v0.9.0', key.line), ['update.json is missing']);
});

test('the shell trusts the recorded update key, and its placeholder stops a release', () => {
  const shell = JSON.parse(fs.readFileSync(path.join(repository, 'apps', 'webapp', 'src-tauri', 'tauri.conf.json'), 'utf8'));
  // Until the owner creates the key (§8 reads `pending`), the shell names a placeholder.
  assert.deepEqual(shellKeyProblems(shell, SIGNER_KEY), ["tauri.conf.json's updater does not name the update key versioning-release.md §8 records"]);
  const file = Buffer.from(`untrusted comment: minisign public key: 2A3CA2CCEC3EE143\n${SIGNER_KEY}\n`).toString('base64');
  const trusting = { plugins: { updater: { ...shell.plugins.updater, pubkey: file } } };
  assert.deepEqual(shellKeyProblems(trusting, SIGNER_KEY), []);
  assert.equal(shellKeyProblems(trusting, updateKey().line).length, 1);
  const elsewhere = { plugins: { updater: { ...trusting.plugins.updater, endpoints: ['https://example.com/latest.json'], requireSignedVersion: false } } };
  assert.deepEqual(shellKeyProblems(elsewhere, SIGNER_KEY), [
    `tauri.conf.json's updater does not ask ${POINTER_ENDPOINT}`,
    "tauri.conf.json's updater does not require the signed version",
  ]);
});

test('the checksum file lists every published file, and the files match it', () => {
  const { dir } = release(updateKey());
  fs.writeFileSync(path.join(dir, CHECKSUMS), checksums(dir));
  const lines = fs.readFileSync(path.join(dir, CHECKSUMS), 'utf8').trim().split('\n');
  assert.equal(lines.length, fs.readdirSync(dir).length - 1);
  for (const line of lines) {
    const [digest, name] = line.split('  ');
    assert.equal(crypto.createHash('sha256').update(fs.readFileSync(path.join(dir, name))).digest('hex'), digest);
  }
  assert.deepEqual(checksumProblems(dir), []);
});

test('a changed, missing or unlisted file does not match the checksum file', () => {
  const { dir } = release(updateKey());
  fs.writeFileSync(path.join(dir, CHECKSUMS), checksums(dir));
  fs.appendFileSync(path.join(dir, 'hubtask-0.9.0-android-universal.apk'), '!');
  fs.rmSync(path.join(dir, 'sbom.cdx.json'));
  fs.writeFileSync(path.join(dir, 'late.txt'), 'late');
  assert.deepEqual(checksumProblems(dir).sort(), [
    'hubtask-0.9.0-android-universal.apk does not match its checksum',
    `late.txt is not in ${CHECKSUMS}`,
    'sbom.cdx.json is listed but missing',
  ]);
});
