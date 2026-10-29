// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The smoke walk on three webviews (UC-SYN-07 check 9, `1.0.0` prerequisite 18): the app as
// installed - the shell built with its `walk` feature, nothing else different - against a real
// server, driven through WebDriver in the system's own webview. Sign in through the workspace's
// consent, open a hub, create an entry, edit it, complete it, sign out; each write is read back
// from the server, so a page that only looks right does not pass.
//
//   node apps/webapp/e2e/webview/smoke.mjs --engine linux|windows|android \
//     --app <the shell's binary, or the APK> --driver <the engine's WebDriver> \
//     --server <hubtask server binary> --migrate <hubtask-migrate binary> \
//     --db <a superuser's DSN of an empty database> [--out <directory>] [--width <px>]
//
// The engine's own driver walks it: WebKitWebDriver for WebKitGTK, msedgedriver for WebView2,
// chromedriver for Android's system WebView. The system's browser is the one thing WebDriver
// cannot reach, so the walk stands in for it (`src-tauri/src/walk.rs`): it takes the address the
// app opens, signs the person in and consents through the API the consent screen calls, and hands
// the code back the way the browser would - the loopback redirect, or Android's deep link.

import { spawn, execFileSync } from 'node:child_process';
import { createServer } from 'node:http';
import { mkdirSync, mkdtempSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { parseArgs } from 'node:util';
import { randomBytes } from 'node:crypto';

import { ENTER, startSession, until } from './webdriver.mjs';

const { values: options } = parseArgs({
  options: {
    engine: { type: 'string' },
    app: { type: 'string' },
    driver: { type: 'string' },
    server: { type: 'string' },
    migrate: { type: 'string' },
    db: { type: 'string' },
    out: { type: 'string' },
    device: { type: 'string', default: 'emulator-5554' },
    // A phone's width on a desktop engine: the walk's compact path, which the Android leg takes.
    width: { type: 'string' },
  },
});
const ENGINES = ['linux', 'windows', 'android'];
for (const name of ['engine', 'app', 'driver', 'server', 'migrate', 'db']) {
  if (!options[name]) fail(`--${name} is missing`);
}
if (!ENGINES.includes(options.engine)) fail(`--engine is one of ${ENGINES.join(', ')}`);
const engine = options.engine;

// Ports of their own, so a server or a driver of something else on the runner is never walked.
const HTTP = 18080;
const OPS = 19090;
const DRIVER = 4445;
const STAND_IN = 4780; // walk.rs's
const DEVTOOLS = 9333; // walk.rs's, WebView2's in a walk build
const PACKAGE = 'eu.hubtask.app';
const SERVER = `http://localhost:${HTTP}`;
const API = `${SERVER}/api/v1`;
const OWNER = { email: 'walker@example.org', password: 'nine grey herons along the river' };
const WORKSPACE = 'Webview walk';
const HUB = 'Errands';
const COLLECTION = 'This week';
const ENTRY = 'Buy stamps';
const EDITED = 'Buy stamps and envelopes';

const out = options.out ?? mkdtempSync(join(tmpdir(), 'hubtask-walk-'));
mkdirSync(out, { recursive: true });
const children = [];
let session;

function fail(message) {
  console.error(`smoke walk: ${message}`);
  process.exit(2);
}

function step(text) {
  console.log(`--- ${engine}: ${text}`);
}

/** A process of the walk's, its output in `out`, stopped when the walk ends. */
function start(name, command, args, env = {}) {
  const child = spawn(command, args, { env: { ...process.env, ...env }, stdio: ['ignore', 'pipe', 'pipe'] });
  const log = [];
  for (const stream of [child.stdout, child.stderr]) stream.on('data', (chunk) => log.push(chunk.toString()));
  const keep = () => writeFileSync(join(out, `${name}.log`), log.join(''));
  child.on('exit', keep);
  children.push({ child, keep });
  return { child, log: () => log.join('') };
}

async function finished(name, command, args, env) {
  const { child, log } = start(name, command, args, env);
  const code = await new Promise((resolve) => child.on('exit', resolve));
  if (code !== 0) throw new Error(`${name} ended with ${code}:\n${log().slice(-2000)}`);
}

async function json(method, path, { token, body } = {}) {
  const response = await fetch(`${API}${path}`, {
    method,
    headers: { ...(token ? { authorization: `Bearer ${token}` } : {}), ...(body ? { 'content-type': 'application/json' } : {}) },
    body: body ? JSON.stringify(body) : undefined,
    signal: AbortSignal.timeout(15_000),
  });
  const text = await response.text();
  if (!response.ok) throw new Error(`${method} ${path} answered ${response.status}: ${text.slice(0, 300)}`);
  return text ? JSON.parse(text) : null;
}

const adb = (...args) => execFileSync('adb', ['-s', options.device, ...args], { encoding: 'utf8', timeout: 120_000 });

// --- The server: migrated, started, set up through its first start's code (UC-INS-01) ----------

async function serverUp() {
  step('the server');
  const appPassword = randomBytes(18).toString('hex');
  await finished('migrate', options.migrate, ['up'], { HUBTASK_DB_DSN: options.db, HUBTASK_DB_APP_PASSWORD: appPassword });
  const appDsn = new URL(options.db);
  appDsn.username = 'hubtask_app';
  appDsn.password = appPassword;
  const data = join(out, 'server-data');
  const server = start('server', options.server, [], {
    HUBTASK_ROLES: 'api',
    HUBTASK_DB_DSN: appDsn.href,
    // Every loopback address: `localhost` is ::1 first on some systems and 127.0.0.1 on others.
    HUBTASK_HTTP_ADDR: `:${HTTP}`,
    HUBTASK_OPS_ADDR: `127.0.0.1:${OPS}`,
    HUBTASK_BASE_URL: SERVER,
    HUBTASK_TENANCY_MODE: 'single',
    HUBTASK_SECRET_KEY: randomBytes(32).toString('base64'),
    HUBTASK_STORAGE_LOCAL_PATH: join(data, 'media'),
    HUBTASK_BACKUP_LOCAL_PATH: join(data, 'backups'),
    // docs/evidence/README.md, "How to walk": a cold page is a burst of requests.
    HUBTASK_RATE_LIMIT_BURST: '600',
  });
  await until('the server to be ready', async () => {
    if (server.child.exitCode !== null) throw new Error(`the server ended with ${server.child.exitCode}`);
    return (await fetch(`http://127.0.0.1:${OPS}/readyz`, { signal: AbortSignal.timeout(5_000) })).ok;
  }, 120_000).catch((error) => { throw new Error(`${error.message}\nthe server said:\n${server.log().slice(-3000)}`); });
  const code = await until('the setup code', () => server.log().match(/\b[0-9A-Z]{4}-[0-9A-Z]{4}-[0-9A-Z]{4}\b/u)?.[0]);
  const setUp = await json('POST', '/auth/installation:set-up', {
    body: { code, display_name: WORKSPACE, owner_email: OWNER.email, password: OWNER.password },
  });
  const token = setUp.access_token;
  // The tour would cover the first screen; it has a walk of its own.
  const me = await json('GET', '/accounts/me', { token });
  await json('PATCH', `/accounts/${me.id}/preferences`, { token, body: { onboarding_completed_at: new Date().toISOString() } });
  const hub = await json('POST', '/containers', { token, body: { type: 'HUB', name: HUB } });
  const collection = await json('POST', '/containers', { token, body: { type: 'COLLECTION', name: COLLECTION, parent_id: hub.id } });
  return { token, collection, name: me.display_name };
}

// --- The system's browser, stood in for ------------------------------------------------------

function standIn() {
  const opened = [];
  const listener = createServer((request, response) => {
    let body = '';
    request.on('data', (chunk) => { body += chunk; });
    request.on('end', () => {
      if (request.method === 'POST' && request.url === '/open') opened.push(body);
      response.writeHead(204).end();
    });
  });
  listener.listen(STAND_IN, '127.0.0.1');
  return { opened, close: () => listener.close() };
}

/** What the person does in the browser: signs in, allows the app, and is sent back to it. */
async function consent(address) {
  const asked = new URL(address);
  const at = (name) => asked.searchParams.get(name);
  const signedIn = await json('POST', '/auth/sessions', { body: { email: OWNER.email, password: OWNER.password } });
  const allowed = await json('POST', '/oauth/authorize', {
    token: signedIn.access_token,
    body: {
      client_id: at('client_id'), redirect_uri: at('redirect_uri'), state: at('state'),
      code_challenge: at('code_challenge'), code_challenge_method: at('code_challenge_method'),
      device: at('device'), platform: at('platform'),
    },
  });
  const back = new URL(at('redirect_uri'));
  back.searchParams.set('code', allowed.code);
  back.searchParams.set('state', allowed.state ?? at('state'));
  if (engine === 'android') {
    adb('shell', `am start -W -a android.intent.action.VIEW -d '${back.href}' ${PACKAGE}`);
  } else {
    const page = await fetch(back, { signal: AbortSignal.timeout(15_000) });
    if (!page.ok) throw new Error(`the app's loopback answered ${page.status}`);
  }
}

// --- The webview, through its engine's driver ------------------------------------------------

async function driverUp() {
  step('the driver');
  const env = { TAURI_WEBVIEW_AUTOMATION: 'true' };
  if (engine === 'windows') await windowsAppUp();
  if (engine === 'android') await androidAppUp();
  start('driver', options.driver, [`--port=${DRIVER}`], env);
  await until('the driver', async () => (await fetch(`http://127.0.0.1:${DRIVER}/status`)).ok, 60_000);
  // WebKitWebDriver starts the app itself; on the other two the walk started it and the driver
  // attaches to its webview.
  const capabilities = {
    linux: { 'webkitgtk:browserOptions': { binary: options.app, args: [] } },
    windows: { browserName: 'webview2', 'ms:edgeOptions': { debuggerAddress: `127.0.0.1:${DEVTOOLS}` } },
    android: { 'goog:chromeOptions': { androidPackage: PACKAGE, androidUseRunningApp: true, androidDeviceSerial: options.device } },
  }[engine];
  const started = await startSession(`http://127.0.0.1:${DRIVER}`, capabilities);
  if (options.width) await started.resize(Number(options.width), 844);
  return started;
}

/** The walk build opens WebView2's debugging port itself (`walk.rs`), so the app runs first. */
async function windowsAppUp() {
  const app = start('app', options.app, []);
  const ended = () => app.child.exitCode ?? app.child.signalCode;
  const version = await until('the webview to answer its driver', async () => {
    if (ended() !== null) return 'ended';
    const answer = await fetch(`http://127.0.0.1:${DEVTOOLS}/json/version`, { signal: AbortSignal.timeout(5_000) });
    return answer.ok && answer.json();
  }, 90_000).catch((error) => { throw new Error(`${error.message}\nthe app said:\n${app.log().slice(-3000)}`); });
  if (version === 'ended') throw new Error(`the app ended with ${ended()}:\n${app.log().slice(-3000)}`);
  // msedgedriver serves one major version of the engine; both named, should they part.
  const driver = execFileSync(options.driver, ['--version'], { encoding: 'utf8', timeout: 30_000 }).trim();
  console.log(`webview ${version.Browser}, ${driver}`);
}

/**
 * Android's WebView opens its debugging socket only once the shell has created it, after the
 * activity started; chromedriver looks once, so the walk waits for the socket and then attaches.
 */
async function androidAppUp() {
  adb('install', '-r', options.app);
  adb('reverse', `tcp:${HTTP}`, `tcp:${HTTP}`);
  adb('reverse', `tcp:${STAND_IN}`, `tcp:${STAND_IN}`);
  adb('shell', 'am', 'start', '-W', '-n', `${PACKAGE}/.MainActivity`);
  try {
    const pid = await until('the app\'s process', () => adb('shell', 'pidof', PACKAGE).trim(), 60_000);
    await until('the app\'s webview to be debuggable', () => adb('shell', 'cat', '/proc/net/unix').includes(`webview_devtools_remote_${pid}`), 90_000);
  } catch (error) {
    const logcat = adb('logcat', '-d', '-t', '2000');
    writeFileSync(join(out, 'logcat.txt'), logcat);
    const telling = logcat.split('\n').filter((line) => /hubtask|chromium|WebView|AndroidRuntime|RustStdout/iu.test(line)).slice(-60);
    throw new Error(`${error.message}\nlogcat:\n${telling.join('\n')}`);
  }
}

async function walk(server, browser) {
  step('sign in');
  const address = await session.wait('input', 'Workspace address', { exact: false });
  await address.type(SERVER);
  await session.click('button', 'Continue');
  await session.wait('h1, h2', 'Continue in your browser');
  const asked = await until('the app to open the browser', () => browser.opened.find((url) => url.includes('/oauth/consent')));
  await consent(asked);

  step('open a hub');
  // A narrow window keeps the tree behind the menu (design-system.md §9).
  const tree = await until('the frame', async () => (await session.find('a, button, [role="treeitem"]', HUB)) ?? (await session.find('button', 'Open the navigation')), 60_000);
  if ((await session.find('a, button, [role="treeitem"]', HUB)) === null) await tree.click();
  await session.click('a, button, [role="treeitem"]', HUB);
  await session.click('a, button, [role="treeitem"]', COLLECTION);

  step('create an entry');
  const add = await until('an add control', async () => (await session.find('main button', 'Add an entry')) ?? (await session.find('nav button', 'Capture')));
  await add.click();
  const field = await session.wait('input, textarea', 'What do you want to remember?', { exact: false });
  await field.type(ENTRY + ENTER);
  await session.wait('main a, main [data-row]', ENTRY, { exact: false });
  const created = await until('the entry on the server', async () => (await items(server)).find((item) => item.title === ENTRY));

  step('edit it');
  await session.click('main a', ENTRY);
  const title = await session.wait('input, textarea', 'Title', { exact: false });
  await title.clear();
  await title.type(EDITED + ENTER);
  await until('the new title on the server', async () => (await items(server)).find((item) => item.id === created.id && item.title === EDITED));

  step('complete it');
  await session.click('input[type="checkbox"], [role="checkbox"]', `Mark ${EDITED} as done`);
  await until('the completion on the server', async () => (await items(server)).find((item) => item.id === created.id && item.completion?.is_completed));

  step('sign out');
  const account = await until('the account menu', async () => (await session.find('button', server.name)) ?? (await session.find('a, button', 'You')));
  await account.click();
  await session.click('button, [role="menuitem"], a', 'Sign out');
  await session.wait('h1, h2', `You are signed out of ${WORKSPACE}`);
}

async function items({ token, collection }) {
  const page = await json('GET', `/items?collection_id=${collection.id}`, { token });
  return page.data ?? page.items ?? [];
}

const browser = standIn();
let failed = false;
try {
  const server = await serverUp();
  session = await driverUp();
  await walk(server, browser);
  step('green');
} catch (error) {
  failed = true;
  console.error(`smoke walk on ${engine} failed: ${error.stack ?? error}`);
  if (session) {
    await session.screenshot().then((png) => writeFileSync(join(out, 'failure.png'), png)).catch(() => {});
    await session.text().then((text) => writeFileSync(join(out, 'failure.txt'), text)).catch(() => {});
    // What the walk could have reached: each control and the name it found it by.
    await session.controls().then((list) => writeFileSync(join(out, 'failure-controls.json'), JSON.stringify(list, null, 1)))
      .catch((error) => writeFileSync(join(out, 'failure-controls.json'), String(error)));
  }
  console.error(`the logs and the last screen are in ${out}`);
} finally {
  await session?.end();
  browser.close();
  for (const { child, keep } of children) {
    child.kill();
    keep();
  }
}
process.exit(failed ? 1 : 0);
