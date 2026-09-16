// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// OAuth2 authorization code with PKCE against the installation's own provider (H-05,
// api-guidelines.md §11) - what the marketplace requires, and what 0.6.0 built for it.
//
// The client is registered with `POST /oauth/clients` by the workspace that installs the app,
// with Zapier's redirect URI, and its identifier and secret are the app's environment
// (`CLIENT_ID`, `CLIENT_SECRET`); the installation's address is the one field the person
// connecting fills in, because Hubtask is self-hosted and has no host of its own.
'use strict';

const { API_VERSION } = require('./hubtask');

const root = '{{bundle.authData.base_url}}/api/' + API_VERSION;

module.exports = {
  type: 'oauth2',
  fields: [
    {
      key: 'base_url',
      label: 'Installation address',
      required: true,
      type: 'string',
      helpText: 'Where your Hubtask runs, such as https://hubtask.example - without /api/v1.',
    },
  ],
  oauth2Config: {
    authorizeUrl: {
      method: 'GET',
      url: `${root}/oauth/authorize`,
      params: {
        client_id: '{{process.env.CLIENT_ID}}',
        state: '{{bundle.inputData.state}}',
        redirect_uri: '{{bundle.inputData.redirect_uri}}',
        response_type: 'code',
        code_challenge: '{{bundle.inputData.code_challenge}}',
        code_challenge_method: 'S256',
        scope: '{{bundle.inputData.scope}}',
      },
    },
    getAccessToken: {
      method: 'POST',
      url: `${root}/oauth/token`,
      body: {
        grant_type: 'authorization_code',
        code: '{{bundle.inputData.code}}',
        client_id: '{{process.env.CLIENT_ID}}',
        client_secret: '{{process.env.CLIENT_SECRET}}',
        redirect_uri: '{{bundle.inputData.redirect_uri}}',
        code_verifier: '{{bundle.inputData.code_verifier}}',
      },
      headers: { 'Content-Type': 'application/x-www-form-urlencoded', Accept: 'application/json' },
    },
    refreshAccessToken: {
      method: 'POST',
      url: `${root}/oauth/token`,
      body: {
        grant_type: 'refresh_token',
        refresh_token: '{{bundle.authData.refresh_token}}',
        client_id: '{{process.env.CLIENT_ID}}',
        client_secret: '{{process.env.CLIENT_SECRET}}',
      },
      headers: { 'Content-Type': 'application/x-www-form-urlencoded', Accept: 'application/json' },
    },
    enablePkce: true,
    autoRefresh: true,
    scope: 'items:read items:write containers:read containers:write automation:manage',
  },
  test: { url: `${root}/accounts/me` },
  connectionLabel: '{{display_name}} at {{bundle.authData.base_url}}',
};
