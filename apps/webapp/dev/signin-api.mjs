// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * A stand-in for the API, so the sign-in work can be walked before the Go half exists.
 *
 * **It is a development tool and it ships nowhere.** Not imported by the bundle, not built, not
 * embedded; the Vite dev server proxies `/api/v1` here and the binary never sees it. It exists for
 * the same reason `e2e/serve.mjs` does - the screens have to be walked by a person before they are
 * walked by a test - and it answers the shapes `api/openapi.yaml` will carry, so that the client
 * is written against the contract rather than against this file.
 *
 * **What it fakes, it fakes honestly.** The refusals are the real ones: one sentence for a wrong
 * password and for an address nobody holds, a `202` where a second factor is owed, the rules read
 * from one place and applied in another. What it does not do is any of the security: there is no
 * hashing, no rate limit and no lockout here, because the point is the screens.
 *
 * Node's own http, no dependency, the way the rest of this repository's tooling is written.
 *
 *     node apps/webapp/dev/signin-api.mjs           # on 8787
 */

import { createServer } from 'node:http';

const PORT = Number(process.env.PORT ?? 8787);

/** The workspace this fake serves, and the three accounts that show the three shapes. */
const WORKSPACE = { display_name: 'Contoso', slug: 'contoso', host: 'contoso.hubtask.eu' };

const ACCOUNTS = {
  'jerome@example.eu': {
    id: '01a0e2e0-0000-7000-8000-000000000001',
    display_name: 'Jérôme Winkel',
    password: 'correct-horse-battery',
    mfa: false,
    stale: false,
  },
  // Has a second factor: any six digits pass, `000000` is refused so the failure can be walked.
  'mara@example.eu': {
    id: '01a0e2e0-0000-7000-8000-000000000011',
    display_name: 'Mara Lind',
    password: 'correct-horse-battery',
    mfa: true,
    stale: false,
  },
  // Right password, rules changed underneath: the `PASSWORD_CHANGE` step.
  'old@example.eu': {
    id: '01a0e2e0-0000-7000-8000-000000000021',
    display_name: 'Sam Okoro',
    password: 'correct-horse-battery',
    mfa: false,
    stale: true,
  },
};

/** The eighteen, as the settings screen writes them and the sign-in screen reads them. */
const setting = (value, installation = value, lock = null) => ({ value, installation, lock });

const policy = {
  password: {
    min_length: setting(14, 12),
    min_lowercase: setting(0),
    min_uppercase: setting(0),
    min_digits: setting(0),
    min_symbols: setting(0),
    min_classes: setting(0),
    max_repeat: setting(null),
    common_passwords: setting(true, true, 'INSTANCE'),
    context_words: setting(true),
    breach_check: setting(false),
    max_age_days: setting(null),
    history_count: setting(4, 0),
    min_age_hours: setting(null),
  },
  mfa_required_for: setting('ADMINS', 'NONE'),
  methods: setting(['PASSWORD', 'OIDC']),
  session: { max_days: setting(30), idle_minutes: setting(null) },
  legal: {
    imprint_url: setting('https://contoso.example/imprint', 'https://hubtask.example/imprint', 'INSTANCE'),
    privacy_url: setting('https://contoso.example/privacy', 'https://hubtask.example/privacy', 'INSTANCE'),
    terms_url: setting('https://contoso.example/terms', ''),
    accessibility_url: setting('', ''),
  },
  rotation_from: null,
};

const PROVIDERS = [
  { id: 'p-entra', display_name: 'Contoso Entra ID', kind: 'MICROSOFT', scope: 'workspace' },
  { id: 'p-google', display_name: 'Google', kind: 'GOOGLE', scope: 'installation' },
  { id: 'p-keycloak', display_name: 'Contoso Lab', kind: 'GENERIC', scope: 'workspace' },
];

/** The corpus the client cannot hold: what a real installation checks server-side. */
const COMMON = new Set(['password', 'passwort', 'letmein', 'hubtask', 'qwertz', 'correcthorsebattery']);
const HISTORY = new Set(['the-one-before-this', 'and-the-one-before-that']);

const flatten = (value) =>
  [...value.normalize('NFKC').toLowerCase()]
    .map((c) => ({ 0: 'o', 1: 'l', 3: 'e', 4: 'a', 5: 's', 7: 't', '@': 'a', $: 's', '!': 'i' })[c] ?? c)
    .join('');

/** The password half of the policy, flattened to the values the sign-in screen is given. */
function passwordRules(withAccount) {
  const p = policy.password;
  return {
    min_length: p.min_length.value,
    min_lowercase: p.min_lowercase.value,
    min_uppercase: p.min_uppercase.value,
    min_digits: p.min_digits.value,
    min_symbols: p.min_symbols.value,
    min_classes: p.min_classes.value,
    max_repeat: p.max_repeat.value,
    common_passwords: p.common_passwords.value,
    context_words: p.context_words.value,
    breach_check: p.breach_check.value,
    // Only somebody who has an account has a history or a current password.
    history_count: withAccount ? p.history_count.value : 0,
    not_current: withAccount,
  };
}

/** What only the server can decide about a candidate. */
function serverViolations(password, { withAccount }) {
  const violations = [];
  const flat = flatten(password);
  if (policy.password.common_passwords.value && [...COMMON].some((word) => flat.includes(word))) {
    violations.push({ rule: 'common' });
  }
  if (withAccount && policy.password.history_count.value > 0 && HISTORY.has(password)) {
    violations.push({ rule: 'history' });
  }
  if (withAccount && password === ACCOUNTS['jerome@example.eu'].password) violations.push({ rule: 'not_current' });
  return violations;
}

const pending = new Map();
const token = () => Math.random().toString(36).slice(2) + Date.now().toString(36);

const json = (response, status, body) => {
  response.writeHead(status, { 'content-type': 'application/json; charset=utf-8' });
  response.end(body === undefined ? '' : JSON.stringify(body));
};

const problem = (response, status, code, detail, fields) =>
  json(response, status, {
    type: 'about:blank',
    title: code,
    status,
    code,
    detail_code: detail ?? code,
    request_id: `req_${token().slice(0, 8)}`,
    ...(fields ? { field_errors: fields } : {}),
  });

async function bodyOf(request) {
  const chunks = [];
  for await (const chunk of request) chunks.push(chunk);
  if (chunks.length === 0) return {};
  try {
    return JSON.parse(Buffer.concat(chunks).toString('utf8'));
  } catch {
    return {};
  }
}

const SESSION = {
  token_type: 'Bearer',
  access_token: 'dev-access',
  access_token_expires_at: new Date(Date.now() + 900_000).toISOString(),
  refresh_token: 'dev-refresh',
  refresh_token_expires_at: new Date(Date.now() + 2_592_000_000).toISOString(),
  session: { id: 'dev-session', created_at: new Date().toISOString(), current: true },
};

const server = createServer(async (request, response) => {
  const url = new URL(request.url, `http://localhost:${PORT}`);
  const path = url.pathname.replace(/^\/api\/v1/, '');
  const method = request.method ?? 'GET';
  const body = method === 'GET' ? {} : await bodyOf(request);
  const bearer = (request.headers.authorization ?? '').replace(/^Bearer /, '');
  const signedIn = bearer !== '';
  // The one credential this fake refuses, so the "your session ended" path can be walked: set
  // `hubtask.bearer` to `expired` in the console and reload.
  if (bearer === 'expired' && !path.startsWith('/auth/')) {
    return problem(response, 401, 'errors.unauthenticated');
  }

  // ── what a signed-out visitor may know ──────────────────────────────────
  if (path === '/auth/sign-in-rules' && method === 'GET') {
    return json(response, 200, {
      workspace_host: WORKSPACE.host,
      methods: policy.methods.value,
      providers: PROVIDERS,
      password: passwordRules(signedIn),
      legal: {
        imprint_url: policy.legal.imprint_url.value,
        privacy_url: policy.legal.privacy_url.value,
        terms_url: policy.legal.terms_url.value,
      },
    });
  }

  if (path === '/auth/password:check' && method === 'POST') {
    const withAccount = signedIn || Boolean(body.pending_token) || Boolean(body.reset_token);
    // No proof, no answer: the route would otherwise tell anybody what is on a blocklist.
    if (!withAccount && !body.invitation_token) return problem(response, 401, 'errors.unauthenticated');
    return json(response, 200, { violations: serverViolations(String(body.password ?? ''), { withAccount }) });
  }

  // ── signing in ──────────────────────────────────────────────────────────
  if (path === '/auth/sessions' && method === 'POST') {
    const account = ACCOUNTS[String(body.email ?? '').toLowerCase()];
    // One refusal, byte for byte the same for a wrong password and an address nobody holds.
    if (!account || account.password !== body.password) {
      return problem(response, 401, 'errors.unauthenticated', 'auth.sign_in_failed');
    }
    if (account.mfa) {
      const handle = token();
      pending.set(handle, { email: account.email ?? '', kind: 'TOTP' });
      return json(response, 202, {
        pending_token: handle,
        methods: ['TOTP', 'RECOVERY'],
        expires_at: new Date(Date.now() + 300_000).toISOString(),
      });
    }
    if (account.stale) {
      const handle = token();
      pending.set(handle, { email: account.email ?? '', kind: 'PASSWORD_CHANGE' });
      return json(response, 202, {
        pending_token: handle,
        methods: ['PASSWORD_CHANGE'],
        expires_at: new Date(Date.now() + 300_000).toISOString(),
        password_rules: passwordRules(true),
      });
    }
    return json(response, 201, SESSION);
  }

  if (path === '/auth/sessions:verify' && method === 'POST') {
    if (!pending.has(String(body.pending_token))) {
      return problem(response, 401, 'errors.unauthenticated', 'auth.mfa_challenge_failed');
    }
    if (body.recovery_code) {
      pending.delete(String(body.pending_token));
      // The number no client has ever read, answered here so the announcement can be walked.
      return json(response, 201, { ...SESSION, recovery_codes_remaining: 3 });
    }
    const code = String(body.code ?? '');
    if (!/^\d{6}$/.test(code) || code === '000000') {
      return problem(response, 401, 'errors.unauthenticated', 'auth.mfa_code_invalid');
    }
    pending.delete(String(body.pending_token));
    return json(response, 201, SESSION);
  }

  if (path === '/auth/sessions:set-password' && method === 'POST') {
    const held = pending.get(String(body.pending_token));
    if (!held) return problem(response, 401, 'errors.unauthenticated', 'auth.mfa_challenge_failed');
    const violations = serverViolations(String(body.password ?? ''), { withAccount: true });
    if (violations.length > 0) {
      return problem(response, 422, 'errors.validation', 'auth.password_refused',
        violations.map((v) => ({ path: '/password', code: `auth.password_rule.${v.rule}` })));
    }
    pending.delete(String(body.pending_token));
    return json(response, 201, SESSION);
  }

  // ── the password over its lifetime ──────────────────────────────────────
  if (path === '/auth/password:forgot' && method === 'POST') {
    // The same answer whether or not the address holds an account.
    console.log(`[dev] a reset link would go to ${body.email}: /reset#token=dev-reset`);
    return json(response, 202, undefined);
  }

  if (path === '/auth/password:reset' && method === 'POST') {
    if (body.token !== 'dev-reset') return problem(response, 401, 'errors.unauthenticated', 'auth.reset_failed');
    const violations = serverViolations(String(body.password ?? ''), { withAccount: true });
    if (violations.length > 0) {
      return problem(response, 422, 'errors.validation', 'auth.password_refused',
        violations.map((v) => ({ path: '/password', code: `auth.password_rule.${v.rule}` })));
    }
    // With a second factor on the account, a reset answers the step rather than a session.
    const handle = token();
    pending.set(handle, { email: 'mara@example.eu', kind: 'TOTP' });
    return json(response, 202, { pending_token: handle, methods: ['TOTP', 'RECOVERY'], expires_at: new Date(Date.now() + 300_000).toISOString() });
  }

  if (path === '/auth/password' && method === 'POST') {
    const violations = serverViolations(String(body.password ?? ''), { withAccount: true });
    if (violations.length > 0) {
      return problem(response, 422, 'errors.validation', 'auth.password_refused',
        violations.map((v) => ({ path: '/password', code: `auth.password_rule.${v.rule}` })));
    }
    return json(response, 204, undefined);
  }

  if (path === '/auth/mfa/recovery:regenerate' && method === 'POST') {
    const codes = Array.from({ length: 10 }, () =>
      `${Math.random().toString(36).slice(2, 6)}-${Math.random().toString(36).slice(2, 6)}`.toUpperCase());
    return json(response, 201, { recovery_codes: codes });
  }

  // ── the workspace, with its policy ──────────────────────────────────────
  if (path === '/tenant' && method === 'GET') {
    return json(response, 200, { ...WORKSPACE, id: 'dev-tenant', status: 'ACTIVE', version: 1, sign_in_policy: policy });
  }
  if (path === '/tenant' && method === 'PATCH') {
    const changes = body.sign_in_policy ?? {};
    for (const [key, value] of Object.entries(changes)) {
      for (const group of [policy.password, policy.session, policy.legal, policy]) {
        const target = group[key] ?? group[key.replace(/^session_/, '')];
        if (target && typeof target === 'object' && 'value' in target) {
          if (target.lock) return problem(response, 422, 'errors.validation', 'errors.forbidden');
          target.value = value;
        }
      }
      if (key === 'rotation_from') policy.rotation_from = new Date().toISOString();
    }
    return json(response, 200, { ...WORKSPACE, id: 'dev-tenant', status: 'ACTIVE', version: 2, sign_in_policy: policy });
  }

  // ── the least the frame reads, so a signed-in screen renders ────────────
  if (path === '/meta/capabilities') {
    return json(response, 200, {
      product_version: 'dev', api_version: 'v1', tenancy_mode: 'multi',
      item_types: [], view_layouts: [], supported_locales: [{ locale: 'en', direction: 'ltr' }],
      roles: [{ role: 'OWNER', permissions: ['READ', 'WRITE_ITEMS', 'STRUCTURE', 'MANAGE_MEMBERS'] }],
      limits: {},
      // The seam the client is gated behind: a server that does not answer this key gets the
      // sign-in screen exactly as it was, with no probe of a route it does not serve.
      features: { sign_in_rules: true },
    });
  }
  if (path === '/accounts/me') {
    return json(response, 200, {
      id: ACCOUNTS['jerome@example.eu'].id, kind: 'USER', status: 'ACTIVE',
      display_name: 'Jérôme Winkel', email: 'jerome@example.eu', locale: 'en',
      // The number the contract has carried since H-02 and no screen had ever shown.
      recovery_codes_remaining: 3,
    });
  }
  if (path === '/meta/health') return json(response, 200, { status: 'operational', version: 'dev', degraded_features: [] });
  if (path === '/stream') {
    response.writeHead(200, { 'content-type': 'text/event-stream' });
    return response.end('retry: 3600000\n\n');
  }
  if (path === '/containers' || path === '/memberships') return json(response, 200, { data: [], page: { next_cursor: null, has_more: false } });
  if (path === '/auth/sessions' && method === 'DELETE') return json(response, 204, undefined);
  // The exchange the seam tries once before it gives up: refused here, so an expired credential
  // ends the session rather than looping.
  if (path === '/auth/sessions:refresh') return problem(response, 401, 'errors.unauthenticated', 'auth.refresh_failed');
  if (path.startsWith('/sync')) return json(response, 200, { changes: [], cursor: 'dev', has_more: false, tombstone_window_days: 90 });

  return problem(response, 404, 'errors.not_found');
});

server.listen(PORT, '127.0.0.1', () => {
  console.log(`[dev] the sign-in fake is on http://127.0.0.1:${PORT}`);
  console.log('[dev] jerome@example.eu / correct-horse-battery   — straight in');
  console.log('[dev] mara@example.eu   / correct-horse-battery   — asks for a code (any six digits, 000000 is refused)');
  console.log('[dev] old@example.eu    / correct-horse-battery   — asks for a new password');
  console.log('[dev] /reset#token=dev-reset                      — the reset link');
});
