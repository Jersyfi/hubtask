// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The credential: an installation's API root and a personal access token, sent as a bearer on
// every request (automation.md §3.3, auth phase 1). Generated beside the node; this file loads it.
'use strict';

const credential = require('./HubtaskApi.credentials.json');

class HubtaskApi {
  constructor() {
    Object.assign(this, credential);
  }
}

module.exports = { HubtaskApi };
