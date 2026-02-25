/**
 * Нормализация ответа API локальных моделей.
 *
 * Зачем:
 * - сервер может вернуть массив или объект-обёртку;
 * - клиенту нужны стабильные поля, иначе рендер model selector падает.
 */
export interface ModelInfo {
  name: string;
  supportsTools: boolean;
  family: string;
  parameterSize: string;
  isCodeModel: boolean;
  suitableRoles: string[];
  roleNotes: { [role: string]: string };
}

const MODEL_RESPONSE_KEYS = ['models', 'items', 'data'] as const;

function pickModelArray(payload: unknown): unknown[] {
  if (Array.isArray(payload)) {
    return payload;
  }

  if (!payload || typeof payload !== 'object') {
    return [];
  }

  const record = payload as Record<string, unknown>;
  for (const key of MODEL_RESPONSE_KEYS) {
    if (Array.isArray(record[key])) {
      return record[key] as unknown[];
    }
  }

  return [];
}

function asModel(item: unknown): ModelInfo | null {
  if (!item || typeof item !== 'object') {
    return null;
  }

  const candidate = item as Record<string, unknown>;
  const name = typeof candidate.name === 'string' ? candidate.name.trim() : '';
  if (!name) {
    return null;
  }

  const suitableRoles = Array.isArray(candidate.suitableRoles)
    ? candidate.suitableRoles.filter((role): role is string => typeof role === 'string')
    : [];

  const rawRoleNotes = candidate.roleNotes;
  const roleNotes = rawRoleNotes && typeof rawRoleNotes === 'object' && !Array.isArray(rawRoleNotes)
    ? Object.fromEntries(
      Object.entries(rawRoleNotes as Record<string, unknown>)
        .filter((entry): entry is [string, string] => typeof entry[0] === 'string' && typeof entry[1] === 'string'),
    )
    : {};

  return {
    name,
    supportsTools: Boolean(candidate.supportsTools),
    family: typeof candidate.family === 'string' ? candidate.family : '',
    parameterSize: typeof candidate.parameterSize === 'string' ? candidate.parameterSize : '',
    isCodeModel: Boolean(candidate.isCodeModel),
    suitableRoles,
    roleNotes,
  };
}

export function normalizeModelList(payload: unknown): ModelInfo[] {
  return pickModelArray(payload)
    .map(asModel)
    .filter((model): model is ModelInfo => model !== null);
}
