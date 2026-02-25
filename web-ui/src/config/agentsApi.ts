/**
 * Нормализация ответа API агентов.
 *
 * Зачем:
 * - backend может вернуть список агентов в разных envelope-формах;
 * - UI не должен ломаться на sortedAgents.map(...) при невалидном payload.
 */
export interface AgentInfo {
  name: string;
  model: string;
  provider: string;
  supportsTools: boolean;
  avatar: string;
  prompt_file?: string;
  prompt: string;
}

const AGENT_RESPONSE_KEYS = ['agents', 'items', 'data'] as const;

function pickAgentArray(payload: unknown): unknown[] {
  if (Array.isArray(payload)) {
    return payload;
  }

  if (!payload || typeof payload !== 'object') {
    return [];
  }

  const record = payload as Record<string, unknown>;
  for (const key of AGENT_RESPONSE_KEYS) {
    if (Array.isArray(record[key])) {
      return record[key] as unknown[];
    }
  }

  return [];
}

function asAgent(item: unknown): AgentInfo | null {
  if (!item || typeof item !== 'object') {
    return null;
  }

  const candidate = item as Record<string, unknown>;
  const name = typeof candidate.name === 'string' ? candidate.name.trim() : '';
  if (!name) {
    return null;
  }

  return {
    name,
    model: typeof candidate.model === 'string' ? candidate.model : '',
    provider: typeof candidate.provider === 'string' ? candidate.provider : 'ollama',
    supportsTools: Boolean(candidate.supportsTools),
    avatar: typeof candidate.avatar === 'string' ? candidate.avatar : '',
    prompt_file: typeof candidate.prompt_file === 'string' ? candidate.prompt_file : undefined,
    prompt: typeof candidate.prompt === 'string' ? candidate.prompt : '',
  };
}

export function normalizeAgentList(payload: unknown): AgentInfo[] {
  return pickAgentArray(payload)
    .map(asAgent)
    .filter((agent): agent is AgentInfo => agent !== null);
}
