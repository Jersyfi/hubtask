// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The governed agents' writes, so every press invalidates the same set (UC-AGT-01 … UC-AGT-04,
 * UC-AGT-07). The reads are subscriptions at the call sites, through `resource`.
 *
 * **Online only.** Agents, their models, held acts and evaluation are not in the replica: each
 * write goes to the server and a refusal is said where it was made.
 */

import type {
  Agent, AgentCase, AgentCaseCreate, AgentHalt, AgentJob, AgentModel, AgentModelChange, AgentModelCreate, AgentRequest, AgentRun, AgentWrite,
} from '@hubtask/sync-engine';

import {
  agentCasesPath, agentEvaluationsPath, agentHaltPath, agentJobsPath, agentModelPath, agentModelsPath, agentPath, agentsPath, meteringPath, requestsPath,
} from './agents.ts';
import { engine } from './engine.ts';

export * from './agents.ts';

const REQUESTS = (['PENDING', 'APPROVED', 'DECLINED', 'FAILED'] as const).map((status) => requestsPath(status));

class Agents {
  register(body: AgentWrite): Promise<Agent> {
    return engine.mutate<Agent>('POST', agentsPath, body, { idempotencyKey: crypto.randomUUID(), invalidates: [agentsPath] });
  }

  change(agent: Agent, body: AgentWrite): Promise<Agent> {
    return engine.mutate<Agent>('PATCH', agentPath(agent.id), body, {
      ifMatch: `"${agent.version}"`,
      invalidates: [agentsPath, agentPath(agent.id)],
    });
  }

  /** Approve, put live, stop or retire: one press, one transition the server checks. */
  press(agentId: string, press: 'approve' | 'activate' | 'suspend' | 'retire'): Promise<Agent> {
    return engine.mutate<Agent>('POST', `${agentPath(agentId)}:${press}`, undefined, { invalidates: [agentsPath, agentPath(agentId)] });
  }

  /** The workspace's kill switch: every agent halted, or released. */
  halt(halted: boolean): Promise<AgentHalt> {
    return engine.mutate<AgentHalt>('PUT', agentHaltPath, { halted }, { invalidates: [agentHaltPath] });
  }

  registerModel(body: AgentModelCreate): Promise<AgentModel> {
    return engine.mutate<AgentModel>('POST', agentModelsPath, body, { idempotencyKey: crypto.randomUUID(), invalidates: [agentModelsPath] });
  }

  /** A change of what the model is asks every agent using it for approval again, so they are stale too. */
  changeModel(model: AgentModel, body: AgentModelChange): Promise<AgentModel> {
    return engine.mutate<AgentModel>('PATCH', agentModelPath(model.id), body, {
      ifMatch: `"${model.version}"`,
      invalidates: [agentModelsPath, agentsPath],
    });
  }

  /** Approving performs the act as the person deciding; what it touched is theirs to read again. */
  decide(request: AgentRequest, approve: boolean): Promise<AgentRequest> {
    return engine.mutate<AgentRequest>('POST', `/agent-requests/${request.id}:decide`, { approve }, {
      invalidates: [...REQUESTS, requestsPath('PENDING', request.agent_id), meteringPath()],
    });
  }

  startJob(agentId: string, targetId: string): Promise<AgentJob> {
    return engine.mutate<AgentJob>('POST', agentJobsPath(agentId), { target_id: targetId }, {
      idempotencyKey: crypto.randomUUID(),
      invalidates: [agentJobsPath(agentId)],
    });
  }

  addCase(agentId: string, body: AgentCaseCreate): Promise<AgentCase> {
    return engine.mutate<AgentCase>('POST', agentCasesPath(agentId), body, {
      idempotencyKey: crypto.randomUUID(),
      invalidates: [agentCasesPath(agentId)],
    });
  }

  /** A shipped agent answers its own set; a CI suggestion and a triage match the hub named. */
  evaluate(agentId: string, hubId?: string): Promise<AgentRun> {
    return engine.mutate<AgentRun>('POST', `${agentEvaluationsPath(agentId)}:run`, hubId ? { hub_id: hubId } : {}, {
      idempotencyKey: crypto.randomUUID(),
      invalidates: [agentEvaluationsPath(agentId)],
    });
  }

  async removeCase(agentId: string, caseId: string): Promise<void> {
    await engine.mutate('DELETE', `${agentCasesPath(agentId)}/${caseId}`, undefined, { invalidates: [agentCasesPath(agentId)] });
  }
}

export const agents = new Agents();
