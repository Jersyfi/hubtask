// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The Hubtask trigger: a webhook subscription over the REST hooks pattern (automation.md §3.1).
//
// n8n calls `checkExists`, `create` and `delete` when a workflow is activated and deactivated;
// each is one request to `/integrations/webhooks`, and the subscription's identifier is kept in
// the workflow's static data so that deactivation deletes exactly what activation created. The
// event types offered are the contract's, generated into events.json beside this file.
//
// The delivery arrives as a CloudEvents document (ADR-0007) signed with `X-Hubtask-Signature`;
// the signature is verified here with the secret the subscription answered once, so a workflow
// only ever runs on what the installation sent. Hand-written: a trigger is procedural in n8n's
// model, and the twenty lines it takes are the same for every event type.
'use strict';

const crypto = require('node:crypto');

const eventTypes = require('./events.json');

class HubtaskTrigger {
  constructor() {
    this.description = {
      displayName: 'Hubtask Trigger',
      name: 'hubtaskTrigger',
      icon: 'fa:tasks',
      group: ['trigger'],
      version: 1,
      description: 'Starts a workflow when something happens in a Hubtask workspace',
      defaults: { name: 'Hubtask Trigger' },
      inputs: [],
      outputs: ['main'],
      credentials: [{ name: 'hubtaskApi', required: true }],
      webhooks: [{ name: 'default', httpMethod: 'POST', responseMode: 'onReceived', path: 'webhook' }],
      properties: [
        {
          displayName: 'Events',
          name: 'events',
          type: 'multiOptions',
          required: true,
          default: [],
          options: eventTypes.map((type) => ({ name: type, value: type })),
          description: 'The event types to subscribe to, as the installation publishes them',
        },
      ],
    };
    this.webhookMethods = {
      default: {
        checkExists: async function () {
          const data = this.getWorkflowStaticData('node');
          if (!data.subscriptionId) return false;
          try {
            await request(this, 'GET', `/integrations/webhooks/${data.subscriptionId}`);
            return true;
          } catch {
            delete data.subscriptionId;
            return false;
          }
        },
        create: async function () {
          const data = this.getWorkflowStaticData('node');
          const created = await request(this, 'POST', '/integrations/webhooks', {
            target_url: this.getNodeWebhookUrl('default'),
            event_types: this.getNodeParameter('events'),
          });
          data.subscriptionId = created.id;
          data.secret = created.secret;
          return true;
        },
        delete: async function () {
          const data = this.getWorkflowStaticData('node');
          if (!data.subscriptionId) return true;
          try {
            await request(this, 'DELETE', `/integrations/webhooks/${data.subscriptionId}`);
          } finally {
            delete data.subscriptionId;
            delete data.secret;
          }
          return true;
        },
      },
    };
  }

  async webhook() {
    const data = this.getWorkflowStaticData('node');
    const request = this.getRequestObject();
    // The bytes the installation sent, not what n8n made of them. A delivery is
    // `application/cloudevents+json` (ADR-0007), which n8n's body parser does not read - it
    // reads `application/json` and the form types, and leaves `body` empty for anything else -
    // so the item is parsed here from the raw body, and the signature is verified over the same
    // bytes it was computed over (issue 723).
    const raw = await rawBodyOf(request);
    const signature = this.getHeaderData()['x-hubtask-signature'];
    if (data.secret) {
      if (!verify(signature, data.secret, raw)) return { webhookResponse: { status: 401 } };
    } else if (this.logger) {
      // A subscription whose secret this workflow does not hold cannot be verified. Said, not
      // hidden: the workflow runs on the strength of its webhook address alone.
      this.logger.warn('Hubtask Trigger: no subscription secret is held for this workflow, so the delivery is not verified');
    }
    let event;
    try {
      event = JSON.parse(raw);
    } catch {
      return { webhookResponse: { status: 400 } };
    }
    return { workflowData: [this.helpers.returnJsonArray(event)] };
  }
}

/**
 * The request body as the sender wrote it. n8n keeps the bytes on `rawBody` - read on demand in
 * the versions that read it lazily - and what it parsed on `body`; only the former can carry a
 * signature, and only the former is there at all for a content type the parser does not know.
 */
async function rawBodyOf(request) {
  if (request.rawBody === undefined && typeof request.readRawBody === 'function') await request.readRawBody();
  const raw = request.rawBody;
  if (Buffer.isBuffer(raw)) return raw.toString('utf8');
  if (typeof raw === 'string') return raw;
  if (typeof request.body === 'string') return request.body;
  return JSON.stringify(request.body ?? {});
}

/** `t=<ts>,v1=<hmac-sha256(secret, ts + "." + body)>`, within a five-minute window (automation.md §3.1). */
function verify(header, secret, body) {
  if (typeof header !== 'string') return false;
  const parts = Object.fromEntries(header.split(',').map((part) => part.split('=')));
  if (!parts.t || !parts.v1) return false;
  if (Math.abs(Date.now() / 1000 - Number(parts.t)) > 300) return false;
  const expected = crypto.createHmac('sha256', secret).update(`${parts.t}.${body}`).digest('hex');
  return expected.length === parts.v1.length && crypto.timingSafeEqual(Buffer.from(expected), Buffer.from(parts.v1));
}

async function request(context, method, path, body) {
  return context.helpers.httpRequestWithAuthentication.call(context, 'hubtaskApi', {
    method,
    url: `${(await context.getCredentials('hubtaskApi')).baseUrl.replace(/\/$/, '')}${path}`,
    json: true,
    body,
  });
}

module.exports = { HubtaskTrigger };
