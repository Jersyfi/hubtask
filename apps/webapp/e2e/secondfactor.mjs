// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Setting up a second factor, walked the same way from the two places it happens - the profile and
// a sign-in the workspace routed into setup - because it is one component and one promise
// (UC-ID-03 checks 1 and 2). Not a test file: the two walks that use it are.

import assert from 'node:assert/strict';

/** The single showing a setup answers: a secret, its URI, and ten codes as the server formats them. */
export const ENROLLMENT = {
  secret: 'JBSWY3DPEHPK3PXPJBSWY3DPEHPK3PXP',
  otpauth_uri: 'otpauth://totp/Hubtask:anna%40contoso.example?secret=JBSWY3DPEHPK3PXPJBSWY3DPEHPK3PXP&issuer=Hubtask',
  recovery_codes: Array.from({ length: 10 }, (_, n) => `K7QM-2XRT-P4ZL-3VW${'ABCDEFGHJK'[n]}`),
};

/**
 * The one walk of UC-ID-03 checks 1 and 2 that both places share: the secret, the confirmation in
 * the sign-in's own code field, then the ten codes once, copyable as one block, and *Continue*
 * unavailable until the reader ticks that they stored them. `sent` is where the stub recorded the requests.
 */
export async function walkSetup(page, sent) {
  await page.getByRole('button', { name: 'Set up a second factor' }).click();
  await page.getByText('JBSW Y3DP').waitFor();

  // The sign-in's code field: one native input drawn as six places, numeric, one-time-code.
  const code = page.getByLabel('Code from your authenticator');
  assert.equal(await code.getAttribute('autocomplete'), 'one-time-code');
  assert.equal(await code.getAttribute('inputmode'), 'numeric');
  await code.fill('123456');
  const places = await page.locator('[class*="place"]:not([class*="places"])').allInnerTexts();
  assert.deepEqual(places, ['1', '2', '3', '4', '5', '6'], 'the confirmation is the code field the sign-in uses');
  await page.getByRole('button', { name: 'Turn it on' }).click();

  // The codes, after the confirmation and once: the one-time panel, hidden until asked for.
  const proceed = page.getByRole('button', { name: 'Continue' });
  await proceed.waitFor();
  assert.deepEqual(sent.confirm?.code, '123456');
  assert.equal(await proceed.isDisabled(), true, 'Continue is available before the codes were stored');
  await page.getByRole('button', { name: 'Show the codes' }).click();
  assert.ok(await page.getByText(ENROLLMENT.recovery_codes[9]).isVisible(), 'the tenth code is shown');
  assert.ok(await page.getByRole('button', { name: 'Copy all ten' }).isVisible(), 'the ten are copyable as one block');
  await page.getByLabel('I have stored these codes. They will not be shown again.').check();
  assert.equal(await proceed.isDisabled(), false, 'ticking did not make Continue available');
  await proceed.click();
}
