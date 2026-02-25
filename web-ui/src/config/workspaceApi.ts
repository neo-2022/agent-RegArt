/**
 * Нормализация ответа API рабочих пространств.
 *
 * Зачем:
 * - Бэкенд может вернуть как «голый» массив, так и объект-обёртку.
 * - UI не должен падать из-за несовпадения формы payload.
 *
 * Edge-cases:
 * - null/undefined/необъектные значения;
 * - элементы без обязательных полей;
 * - числовые значения в строковом формате.
 */
export interface WorkspaceInfo {
  ID: number;
  Name: string;
  Path: string;
}

const WORKSPACE_RESPONSE_KEYS = ['workspaces', 'items', 'data'] as const;

function asWorkspace(item: unknown): WorkspaceInfo | null {
  if (!item || typeof item !== 'object') {
    return null;
  }

  const candidate = item as Record<string, unknown>;
  const rawId = candidate.ID;
  const rawName = candidate.Name;
  const rawPath = candidate.Path;

  const normalizedId = Number(rawId);
  if (!Number.isFinite(normalizedId) || typeof rawName !== 'string' || typeof rawPath !== 'string') {
    return null;
  }

  return {
    ID: normalizedId,
    Name: rawName,
    Path: rawPath,
  };
}

function pickWorkspaceArray(payload: unknown): unknown[] {
  if (Array.isArray(payload)) {
    return payload;
  }

  if (!payload || typeof payload !== 'object') {
    return [];
  }

  const record = payload as Record<string, unknown>;
  for (const key of WORKSPACE_RESPONSE_KEYS) {
    if (Array.isArray(record[key])) {
      return record[key] as unknown[];
    }
  }

  return [];
}

export function normalizeWorkspaceList(payload: unknown): WorkspaceInfo[] {
  return pickWorkspaceArray(payload)
    .map(asWorkspace)
    .filter((workspace): workspace is WorkspaceInfo => workspace !== null);
}
