// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The service desk's paths and writes for the team (UC-SVC-01 to 07): business calendars, SLA
 * policies, canned replies and the report at the workspace, a hub's own calendars, an entry's SLA goals, a reply filled
 * for an entry, and a macro pressed on one. A definition is a whole replacement under its key.
 */

import type {
  BusinessCalendar, BusinessCalendarDefine, RenderedReply, ReplyTemplate, ReplyTemplateDefine, RequesterOrganisation,
  RequesterOrganisationDefine, SlaPolicy, SlaPolicyDefine,
} from '@hubtask/sync-engine';

import { engine } from './engine.ts';

export const calendarsPath = '/business-calendars';
export const policiesPath = '/sla-policies';
export const repliesPath = '/reply-templates';
export const organisationsPath = '/requester-organisations';
export const hubCalendarsPath = (hubId: string) => `/hubs/${hubId}/business-calendars`;
export const entrySlaPath = (itemId: string) => `/items/${itemId}/sla`;
export const slaReportPath = (from: string, to: string) => `/sla-report?from=${encodeURIComponent(from)}&to=${encodeURIComponent(to)}`;

class Desk {
  defineCalendar(key: string, body: BusinessCalendarDefine): Promise<BusinessCalendar> {
    return engine.mutate<BusinessCalendar>('PUT', `${calendarsPath}/${key}`, body, { invalidates: [calendarsPath] });
  }

  deleteCalendar(key: string): Promise<unknown> {
    return engine.mutate('DELETE', `${calendarsPath}/${key}`, undefined, { invalidates: [calendarsPath] });
  }

  /** A hub's own version of the workspace calendar of the key (UC-SVC-01 check 5). */
  defineHubCalendar(hubId: string, key: string, body: BusinessCalendarDefine): Promise<BusinessCalendar> {
    return engine.mutate<BusinessCalendar>('PUT', `${hubCalendarsPath(hubId)}/${key}`, body, { invalidates: [hubCalendarsPath(hubId)] });
  }

  deleteHubCalendar(hubId: string, key: string): Promise<unknown> {
    return engine.mutate('DELETE', `${hubCalendarsPath(hubId)}/${key}`, undefined, { invalidates: [hubCalendarsPath(hubId)] });
  }

  definePolicy(key: string, body: SlaPolicyDefine): Promise<SlaPolicy> {
    return engine.mutate<SlaPolicy>('PUT', `${policiesPath}/${key}`, body, { invalidates: [policiesPath] });
  }

  deletePolicy(key: string): Promise<unknown> {
    return engine.mutate('DELETE', `${policiesPath}/${key}`, undefined, { invalidates: [policiesPath] });
  }

  defineReply(key: string, body: ReplyTemplateDefine): Promise<ReplyTemplate> {
    return engine.mutate<ReplyTemplate>('PUT', `${repliesPath}/${key}`, body, { invalidates: [repliesPath] });
  }

  deleteReply(key: string): Promise<unknown> {
    return engine.mutate('DELETE', `${repliesPath}/${key}`, undefined, { invalidates: [repliesPath] });
  }

  /** A portal organisation by its mail domains (UC-SVC-11). */
  defineOrganisation(key: string, body: RequesterOrganisationDefine): Promise<RequesterOrganisation> {
    return engine.mutate<RequesterOrganisation>('PUT', `${organisationsPath}/${key}`, body, { invalidates: [organisationsPath] });
  }

  deleteOrganisation(key: string): Promise<unknown> {
    return engine.mutate('DELETE', `${organisationsPath}/${key}`, undefined, { invalidates: [organisationsPath] });
  }

  /** A canned reply filled for the entry; it sends nothing. */
  async render(itemId: string, key: string): Promise<RenderedReply> {
    const state = await engine.refresh<RenderedReply>({ path: `/items/${itemId}/reply-templates/${key}` });
    if (state.status === 'ready') return state.data;
    throw state.status === 'failed' ? state.error : new Error('reply_templates.not_rendered');
  }

  /** Presses a manual rule on the entry: a macro (UC-SVC-06 check 4). */
  press(ruleId: string, itemId: string): Promise<unknown> {
    return engine.mutate('POST', `/automation/rules/${ruleId}:trigger`, { item_id: itemId }, {
      idempotencyKey: crypto.randomUUID(), invalidates: [`/items/${itemId}`],
    });
  }
}

export const desk = new Desk();
