// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The Hubtask node: a declarative n8n node whose whole description is generated from the
// contract (P-04, ADR-0058). There is no code per operation - the routing in the description
// says what each one requests - and this file only loads it. CommonJS, because that is what n8n
// requires of a community node package.
'use strict';

const description = require('./Hubtask.node.json');

class Hubtask {
  constructor() {
    this.description = description;
  }
}

module.exports = { Hubtask };
