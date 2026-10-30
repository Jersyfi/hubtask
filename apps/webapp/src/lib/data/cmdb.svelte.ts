// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * A hub's configuration items: the reads the *Assets* screen asks once and the writes, so every
 * write invalidates the same set (UC-AST-01 … UC-AST-06). The lists are subscriptions at the call
 * sites. A CI is written with its version; a run, a hold or a duplicate has none to keep. A
 * connector (UC-AST-07, UC-AST-08) is defined with its version and never deleted. An entry's
 * affected CIs, a campaign's answers, a group, an asset and a software alias (UC-AST-09 … 13) each
 * invalidate what they change: an affected CI the entry's list, an answer its campaign and the CI's
 * review, an asset move the CI's stage.
 */

import type {
  CiConnector, CiConnectorDefine, CiConnectorRun, CiBacklinkList, CiClass, CiClassDefine, CiRelationType, CiRelationTypeDefine, CiSource, CiSourceDefine, CiCompliance, CiDuplicateResolved, CiFileImport, CiImpact, CiRelation, CiRun, ConfigurationItem, ConfigurationItemCreate,
  ConfigurationItemList, ConfigurationItemUpdate, CiAsset, CiAssetRegister, CiAttestation, CiCampaign, CiCampaignStart, CiGroup, CiGroupDefine,
  EntryCis, SoftwareAlias, SoftwareAliasDefine,
} from '@hubtask/sync-engine';

import {
  assetsPath, attestPath, campaignClosePath, campaignPath, campaignsPath, ciAssetMovePath, ciAssetPath, entryCiPath, entryCisPath, groupPath,
  groupsPath, serviceMapPath, softwareAliasPath, softwareAliasesPath, type AssetState, type DisposalMethod,
} from './cmdb.ts';
import { backlinksPath, ciPath, connectorCredentialPath, connectorPath, connectorRunPath, connectorsPath, cisPath, classesPath, duplicatesPath, healthPath, holdsPath, impactPath, relationTypesPath, runPath, runsPath, sourcesPath } from './cmdb.ts';
import { engine } from './engine.ts';

export * from './cmdb.ts';

const inventory = (hubId: string) => [cisPath(hubId), healthPath(hubId)];
const reconciled = (hubId: string) => [...inventory(hubId), runsPath(hubId), holdsPath(hubId), duplicatesPath(hubId)];

class Cmdb {
  /** The page after `after`, for the inventory's *Load more*: the list answers `next`, not a page cursor. */
  async page(hubId: string, after: string, ofClass?: string): Promise<ConfigurationItemList> {
    const state = await engine.refresh<ConfigurationItemList>({ path: cisPath(hubId, { after, ...(ofClass ? { class: ofClass } : {}) }) });
    if (state.status === 'failed') throw state.error;
    return state.status === 'ready' ? state.data : { data: [] };
  }

  async impact(ciId: string, direction: 'DOWNSTREAM' | 'UPSTREAM'): Promise<CiImpact> {
    const state = await engine.refresh<CiImpact>({ path: impactPath(ciId, direction) });
    if (state.status !== 'ready') throw state.status === 'failed' ? state.error : new Error('impact');
    return state.data;
  }

  /** What points at a CI and the reader may see. */
  async backlinks(ciId: string): Promise<CiBacklinkList['data']> {
    const state = await engine.refresh<CiBacklinkList>({ path: backlinksPath(ciId) });
    if (state.status === 'failed') throw state.error;
    return state.status === 'ready' ? state.data.data : [];
  }

  async run(runId: string): Promise<CiRun> {
    const state = await engine.refresh<CiRun>({ path: runPath(runId) });
    if (state.status !== 'ready') throw state.status === 'failed' ? state.error : new Error('run');
    return state.data;
  }

  /** One CI read for a side-by-side view; failures are left out, the duplicate still shown. */
  async read(ciId: string): Promise<ConfigurationItem | undefined> {
    const state = await engine.refresh<ConfigurationItem>({ path: ciPath(ciId) });
    return state.status === 'ready' ? state.data : undefined;
  }

  create(hubId: string, body: ConfigurationItemCreate): Promise<ConfigurationItem> {
    return engine.mutate<ConfigurationItem>('POST', cisPath(hubId), body, {
      idempotencyKey: crypto.randomUUID(),
      invalidates: reconciled(hubId),
    });
  }

  update(ci: ConfigurationItem, body: ConfigurationItemUpdate): Promise<ConfigurationItem> {
    return engine.mutate<ConfigurationItem>('PATCH', ciPath(ci.id), body, {
      ifMatch: `"${ci.version}"`,
      invalidates: [ciPath(ci.id), ...inventory(ci.hub_id)],
    });
  }

  setCompliance(ci: ConfigurationItem, body: CiCompliance): Promise<ConfigurationItem> {
    return engine.mutate<ConfigurationItem>('PUT', `${ciPath(ci.id)}/compliance`, body, {
      ifMatch: `"${ci.version}"`,
      invalidates: [ciPath(ci.id), ...inventory(ci.hub_id)],
    });
  }

  relate(hubId: string, sourceId: string, targetId: string, type: string): Promise<CiRelation> {
    return engine.mutate<CiRelation>('POST', '/ci-relations', { source_ci_id: sourceId, target_ci_id: targetId, type }, {
      idempotencyKey: crypto.randomUUID(),
      invalidates: [ciPath(sourceId), ciPath(targetId), healthPath(hubId)],
    });
  }

  unrelate(hubId: string, relation: CiRelation): Promise<{ id: string; removed: boolean }> {
    return engine.mutate('DELETE', `/ci-relations/${relation.id}`, undefined, {
      invalidates: [ciPath(relation.source_ci_id), ciPath(relation.target_ci_id), healthPath(hubId)],
    });
  }

  /** A file through the reconciliation; a dry run invalidates nothing, because it wrote nothing. */
  importFile(hubId: string, body: CiFileImport): Promise<CiRun> {
    return engine.mutate<CiRun>('POST', `/hubs/${hubId}/cis:import`, body, {
      ...(body.dry_run ? {} : { idempotencyKey: crypto.randomUUID() }),
      invalidates: body.dry_run ? [] : reconciled(hubId),
    });
  }

  dismissHold(hubId: string, holdId: string): Promise<{ id: string; dismissed: boolean }> {
    return engine.mutate('POST', `/ci-holds/${holdId}:dismiss`, undefined, { invalidates: [holdsPath(hubId)] });
  }

  /** With a survivor the others merge into it; without one they are recorded as distinct. */
  resolveDuplicate(hubId: string, duplicateId: string, survivorId?: string): Promise<CiDuplicateResolved> {
    return engine.mutate<CiDuplicateResolved>('POST', `/ci-duplicates/${duplicateId}:resolve`, survivorId ? { survivor_id: survivorId } : {}, {
      invalidates: reconciled(hubId),
    });
  }

  /** A connector added under its key, or changed with the version last read. */
  defineConnector(hubId: string, key: string, body: CiConnectorDefine, version?: number): Promise<CiConnector> {
    return engine.mutate<CiConnector>('PUT', connectorPath(hubId, key), body, {
      ...(version ? { ifMatch: `"${version}"` } : {}),
      invalidates: [connectorsPath(hubId)],
    });
  }

  /**
   * A pull's credential set, or an intake's secret drawn; the answer carries an intake's `secret`
   * once, so the caller shows it and keeps nothing.
   */
  setConnectorCredential(connector: CiConnector, credential?: string): Promise<CiConnector> {
    return engine.mutate<CiConnector>('PUT', connectorCredentialPath(connector.id), credential ? { credential } : {}, {
      invalidates: [connectorsPath(connector.hub_id)],
    });
  }

  /** A pull now; a dry run wrote nothing, so it invalidates only the connector's outcome. */
  runConnector(connector: CiConnector, options: { full?: boolean; dryRun?: boolean }): Promise<CiConnectorRun> {
    return engine.mutate<CiConnectorRun>('POST', connectorRunPath(connector.id), { full: options.full ?? false, dry_run: options.dryRun ?? false }, {
      invalidates: options.dryRun ? [connectorsPath(connector.hub_id)] : [connectorsPath(connector.hub_id), ...reconciled(connector.hub_id)],
    });
  }

  /** An own class added, or changed with the version last read. */
  defineClass(key: string, body: CiClassDefine, version?: number): Promise<CiClass> {
    return engine.mutate<CiClass>('PUT', `${classesPath}/${encodeURIComponent(key)}`, body, {
      ...(version ? { ifMatch: `"${version}"` } : {}),
      invalidates: [classesPath],
    });
  }

  defineRelationType(key: string, body: CiRelationTypeDefine, version?: number): Promise<CiRelationType> {
    return engine.mutate<CiRelationType>('PUT', `${relationTypesPath}/${encodeURIComponent(key)}`, body, {
      ...(version ? { ifMatch: `"${version}"` } : {}),
      invalidates: [relationTypesPath],
    });
  }

  defineSource(key: string, body: CiSourceDefine, version?: number): Promise<CiSource> {
    return engine.mutate<CiSource>('PUT', `${sourcesPath}/${encodeURIComponent(key)}`, body, {
      ...(version ? { ifMatch: `"${version}"` } : {}),
      invalidates: [sourcesPath],
    });
  }

  affect(itemId: string, ciId: string): Promise<EntryCis> {
    return engine.mutate<EntryCis>('PUT', entryCiPath(itemId, ciId), undefined, { invalidates: [entryCisPath(itemId)] });
  }

  unaffect(itemId: string, ciId: string): Promise<EntryCis> {
    return engine.mutate<EntryCis>('DELETE', entryCiPath(itemId, ciId), undefined, { invalidates: [entryCisPath(itemId)] });
  }

  startCampaign(hubId: string, body: CiCampaignStart): Promise<CiCampaign> {
    return engine.mutate<CiCampaign>('POST', campaignsPath(hubId), body, {
      idempotencyKey: crypto.randomUUID(),
      invalidates: [campaignsPath(hubId)],
    });
  }

  attest(campaign: CiCampaign, ciId: string, verdict: 'CONFIRMED' | 'DISPUTED'): Promise<CiAttestation> {
    return engine.mutate<CiAttestation>('PUT', attestPath(campaign.id, ciId), { verdict }, {
      invalidates: [campaignPath(campaign.id), campaignsPath(campaign.hub_id), ciPath(ciId), healthPath(campaign.hub_id)],
    });
  }

  closeCampaign(campaign: CiCampaign): Promise<CiCampaign> {
    return engine.mutate<CiCampaign>('POST', campaignClosePath(campaign.id), undefined, {
      invalidates: [campaignPath(campaign.id), campaignsPath(campaign.hub_id)],
    });
  }

  /** A group added under its key, or changed with the version last read. */
  defineGroup(hubId: string, key: string, body: CiGroupDefine, version?: number): Promise<CiGroup> {
    return engine.mutate<CiGroup>('PUT', groupPath(hubId, key), body, {
      ...(version ? { ifMatch: `"${version}"` } : {}),
      invalidates: [groupsPath(hubId), '/ci-groups/', serviceMapPath(hubId)],
    });
  }

  /** A CI's asset written; a new one without a version, a known one with the version last read. */
  registerAsset(ci: Pick<ConfigurationItem, 'id' | 'hub_id'>, body: CiAssetRegister, version?: number): Promise<CiAsset> {
    return engine.mutate<CiAsset>('PUT', ciAssetPath(ci.id), body, {
      ...(version ? { ifMatch: `"${version}"` } : {}),
      invalidates: [ciAssetPath(ci.id), assetsPath(ci.hub_id), ciPath(ci.id), cisPath(ci.hub_id)],
    });
  }

  moveAsset(asset: CiAsset, state: AssetState, disposal?: { method: DisposalMethod; certificate: string }): Promise<CiAsset> {
    return engine.mutate<CiAsset>('POST', ciAssetMovePath(asset.ci_id), { state, ...(disposal ? { disposal } : {}) }, {
      ifMatch: `"${asset.version}"`,
      invalidates: [ciAssetPath(asset.ci_id), assetsPath(asset.hub_id), ciPath(asset.ci_id), cisPath(asset.hub_id), healthPath(asset.hub_id)],
    });
  }

  defineAlias(body: SoftwareAliasDefine): Promise<SoftwareAlias> {
    return engine.mutate<SoftwareAlias>('POST', softwareAliasesPath, body, {
      invalidates: [softwareAliasesPath, '/hubs/*/software'],
    });
  }

  removeAlias(alias: SoftwareAlias): Promise<{ id: string; removed: boolean }> {
    return engine.mutate('DELETE', softwareAliasPath(alias.id), undefined, { invalidates: [softwareAliasesPath, '/hubs/*/software'] });
  }
}

export const cmdb = new Cmdb();
