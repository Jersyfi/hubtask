// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The least of a W3C WebDriver client the smoke walk needs, over `fetch`: each engine's own driver
// (WebKitWebDriver, msedgedriver, chromedriver) speaks the same protocol, so one client walks all
// three webviews, and no client library joins the shell's dependencies (F7 Decision 4).

const ELEMENT = 'element-6066-11e4-a52e-4f735466cecf';

export class WebDriverError extends Error {}

async function call(base, method, path, body) {
  const response = await fetch(`${base}${path}`, {
    method,
    headers: body === undefined ? {} : { 'content-type': 'application/json' },
    body: body === undefined ? undefined : JSON.stringify(body),
    signal: AbortSignal.timeout(120_000),
  });
  const answer = await response.json().catch(() => ({}));
  if (!response.ok) {
    const { error, message } = answer.value ?? {};
    throw new WebDriverError(`${method} ${path}: ${error ?? response.status} ${message ?? ''}`.trim());
  }
  return answer.value;
}

/** One session on the driver at `base` (`http://127.0.0.1:4444`). */
export async function startSession(base, capabilities) {
  const value = await call(base, 'POST', '/session', { capabilities: { alwaysMatch: capabilities } });
  return new Session(base, value.sessionId);
}

/** Whatever answers within `ms`, or the failure naming what was awaited. */
export async function until(what, probe, ms = 30_000) {
  const deadline = Date.now() + ms;
  let last;
  while (Date.now() < deadline) {
    try {
      const found = await probe();
      if (found) return found;
    } catch (error) {
      last = error;
    }
    await new Promise((resolve) => setTimeout(resolve, 250));
  }
  throw new WebDriverError(`waited ${ms} ms for ${what}${last ? ` (${last.message})` : ''}`);
}

// Finds a visible element in the page, as a person would by what it says: a control by its
// accessible name (label, aria-label, text), optionally narrowed by a CSS selector.
const FIND = `
  const [selector, name, exact] = arguments;
  const visible = (el) => { const r = el.getBoundingClientRect(); return r.width > 0 && r.height > 0 && getComputedStyle(el).visibility !== 'hidden'; };
  const words = (s) => (s ?? '').replace(/\\s+/g, ' ').trim();
  const nameOf = (el) => {
    const labelled = el.getAttribute('aria-labelledby');
    if (labelled) return words(labelled.split(' ').map((id) => document.getElementById(id)?.textContent).join(' '));
    if (el.getAttribute('aria-label')) return words(el.getAttribute('aria-label'));
    if (el.labels && el.labels.length) return words(el.labels[0].textContent);
    if (el.placeholder) return words(el.placeholder);
    return words(el.textContent);
  };
  const matches = (el) => { const n = nameOf(el); return exact ? n === name : n.includes(name); };
  return [...document.querySelectorAll(selector)].find((el) => visible(el) && (name === null || matches(el))) ?? null;
`;

export class Session {
  constructor(base, id) {
    this.base = base;
    this.id = id;
  }

  #call(method, path, body) {
    return call(this.base, method, `/session/${this.id}${path}`, body);
  }

  script(source, ...args) {
    return this.#call('POST', '/execute/sync', { script: source, args });
  }

  /** The first visible element under `selector` whose name is `name` (all of it when `exact`). */
  async find(selector, name = null, { exact = true } = {}) {
    const found = await this.script(FIND, selector, name, exact);
    return found?.[ELEMENT] ? new Element(this, found[ELEMENT]) : null;
  }

  /** As `find`, waiting for it to appear. */
  wait(selector, name = null, { exact = true, ms = 30_000 } = {}) {
    return until(`${selector}${name === null ? '' : ` named "${name}"`}`, () => this.find(selector, name, { exact }), ms);
  }

  /** Finds and clicks it, again while it is still moving into place (a sheet sliding in). */
  click(selector, name = null, { exact = true, ms = 30_000 } = {}) {
    return until(`to click ${selector}${name === null ? '' : ` named "${name}"`}`, async () => {
      const element = await this.find(selector, name, { exact });
      if (element === null) return false;
      await element.click();
      return true;
    }, ms);
  }

  /** Waits until nothing under `selector` is named `name` any longer. */
  async gone(selector, name, { exact = true, ms = 30_000 } = {}) {
    await until(`${selector} named "${name}" to go`, async () => (await this.find(selector, name, { exact })) === null, ms);
  }

  /** Every control on the page with the name `find` reads, for a failure's record. */
  controls() {
    return this.script(`${FIND.replace(/return \[\.\.\.[^\n]*\n/u, '')}
      return [...document.querySelectorAll('input, textarea, button, a, [role]')].map((el) => [el.tagName, el.getAttribute('role'), nameOf(el), visible(el)]);`, '', null, false);
  }

  resize(width, height) {
    return this.#call('POST', '/window/rect', { width, height });
  }

  text() {
    return this.script('return document.body.innerText;');
  }

  async screenshot() {
    return Buffer.from(await this.#call('GET', '/screenshot'), 'base64');
  }

  async end() {
    await this.#call('DELETE', '').catch(() => {});
  }

  elementCall(method, id, path, body) {
    return this.#call(method, `/element/${id}${path}`, body);
  }
}

export class Element {
  constructor(session, id) {
    this.session = session;
    this.id = id;
  }

  click() {
    return this.session.elementCall('POST', this.id, '/click', {});
  }

  /** Types `text` as keys; `` is Enter. */
  type(text) {
    return this.session.elementCall('POST', this.id, '/value', { text });
  }

  clear() {
    return this.session.elementCall('POST', this.id, '/clear', {});
  }

  value() {
    return this.session.script('return arguments[0].value;', { [ELEMENT]: this.id });
  }

  checked() {
    return this.session.script('return arguments[0].checked === true || arguments[0].getAttribute("aria-checked") === "true";', { [ELEMENT]: this.id });
  }
}

export const ENTER = '';
