/**
 * Нормализация ответа API провайдеров моделей.
 *
 * На практике backend может отдавать разные envelope-формы.
 * Клиент обязан валидировать payload на границе, чтобы не падать на providers.find/filter/map.
 */
export interface ProviderGuideInfo {
  how_to_connect: string;
  how_to_choose: string;
  how_to_pay: string;
  how_to_balance: string;
}

export interface ModelDetailInfo {
  id: string;
  is_available: boolean;
  pricing_info: string;
  activation_hint: string;
}

export interface ProviderInfo {
  name: string;
  enabled: boolean;
  hasKey: boolean;
  models: string[];
  guide?: ProviderGuideInfo;
  models_detail?: ModelDetailInfo[];
}

const PROVIDER_RESPONSE_KEYS = ['providers', 'items', 'data'] as const;

function pickProviderArray(payload: unknown): unknown[] {
  if (Array.isArray(payload)) {
    return payload;
  }

  if (!payload || typeof payload !== 'object') {
    return [];
  }

  const record = payload as Record<string, unknown>;
  for (const key of PROVIDER_RESPONSE_KEYS) {
    if (Array.isArray(record[key])) {
      return record[key] as unknown[];
    }
  }

  return [];
}

function asModelDetails(raw: unknown): ModelDetailInfo[] {
  if (!Array.isArray(raw)) {
    return [];
  }

  return raw
    .filter((item): item is Record<string, unknown> => Boolean(item) && typeof item === 'object')
    .map((item) => ({
      id: typeof item.id === 'string' ? item.id : '',
      is_available: Boolean(item.is_available),
      pricing_info: typeof item.pricing_info === 'string' ? item.pricing_info : '',
      activation_hint: typeof item.activation_hint === 'string' ? item.activation_hint : '',
    }))
    .filter((detail) => detail.id.length > 0);
}

function asProvider(item: unknown): ProviderInfo | null {
  if (!item || typeof item !== 'object') {
    return null;
  }

  const record = item as Record<string, unknown>;
  const name = typeof record.name === 'string' ? record.name.trim() : '';
  if (!name) {
    return null;
  }

  const models = Array.isArray(record.models)
    ? record.models.filter((model): model is string => typeof model === 'string')
    : [];

  const guideRaw = record.guide;
  const guide = guideRaw && typeof guideRaw === 'object' && !Array.isArray(guideRaw)
    ? {
      how_to_connect: typeof (guideRaw as Record<string, unknown>).how_to_connect === 'string' ? (guideRaw as Record<string, unknown>).how_to_connect as string : '',
      how_to_choose: typeof (guideRaw as Record<string, unknown>).how_to_choose === 'string' ? (guideRaw as Record<string, unknown>).how_to_choose as string : '',
      how_to_pay: typeof (guideRaw as Record<string, unknown>).how_to_pay === 'string' ? (guideRaw as Record<string, unknown>).how_to_pay as string : '',
      how_to_balance: typeof (guideRaw as Record<string, unknown>).how_to_balance === 'string' ? (guideRaw as Record<string, unknown>).how_to_balance as string : '',
    }
    : undefined;

  return {
    name,
    enabled: Boolean(record.enabled),
    hasKey: Boolean(record.hasKey),
    models,
    guide,
    models_detail: asModelDetails(record.models_detail),
  };
}

export function normalizeProviderList(payload: unknown): ProviderInfo[] {
  return pickProviderArray(payload)
    .map(asProvider)
    .filter((provider): provider is ProviderInfo => provider !== null);
}
