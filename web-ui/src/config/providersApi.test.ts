import { describe, expect, it } from 'vitest';

import { normalizeProviderList } from './providersApi';

describe('normalizeProviderList', () => {
  const provider = {
    name: 'openai',
    enabled: true,
    hasKey: true,
    models: ['gpt-4o'],
    models_detail: [{ id: 'gpt-4o', is_available: true, pricing_info: '$$', activation_hint: 'ok' }],
  };

  it('поддерживает массив', () => {
    expect(normalizeProviderList([provider])).toEqual([provider]);
  });

  it('поддерживает обёртку providers', () => {
    expect(normalizeProviderList({ providers: [provider] })).toEqual([provider]);
  });

  it('фильтрует невалидные элементы', () => {
    expect(normalizeProviderList({ data: [{ bad: true }, provider] })).toEqual([provider]);
  });

  it('заполняет безопасные дефолты при частичной записи', () => {
    expect(normalizeProviderList([{ name: 'ollama' }])).toEqual([
      { name: 'ollama', enabled: false, hasKey: false, models: [], guide: undefined, models_detail: [] },
    ]);
  });
});
