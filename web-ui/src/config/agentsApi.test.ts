import { describe, expect, it } from 'vitest';

import { normalizeAgentList } from './agentsApi';

describe('normalizeAgentList', () => {
  const baseAgent = {
    name: 'admin',
    model: 'qwen2.5:7b',
    provider: 'ollama',
    supportsTools: true,
    avatar: '',
    prompt: 'Ты помощник',
  };

  it('поддерживает массив', () => {
    expect(normalizeAgentList([baseAgent])).toEqual([baseAgent]);
  });

  it('поддерживает обёртку agents', () => {
    expect(normalizeAgentList({ agents: [baseAgent] })).toEqual([baseAgent]);
  });

  it('фильтрует невалидные записи', () => {
    expect(normalizeAgentList({ data: [{ bad: true }, baseAgent] })).toEqual([baseAgent]);
  });

  it('проставляет безопасные дефолты для необязательных полей', () => {
    expect(normalizeAgentList([{ name: 'admin' }])).toEqual([
      {
        name: 'admin',
        model: '',
        provider: 'ollama',
        supportsTools: false,
        avatar: '',
        prompt_file: undefined,
        prompt: '',
      },
    ]);
  });
});
