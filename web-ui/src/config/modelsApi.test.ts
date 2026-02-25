import { describe, expect, it } from 'vitest';

import { normalizeModelList } from './modelsApi';

describe('normalizeModelList', () => {
  const fullModel = {
    name: 'qwen2.5:7b',
    supportsTools: true,
    family: 'qwen',
    parameterSize: '7b',
    isCodeModel: false,
    suitableRoles: ['admin'],
    roleNotes: { admin: 'ok' },
  };

  it('принимает массив', () => {
    expect(normalizeModelList([fullModel])).toEqual([fullModel]);
  });

  it('принимает обёртку data', () => {
    expect(normalizeModelList({ data: [fullModel] })).toEqual([fullModel]);
  });

  it('фильтрует невалидные элементы', () => {
    expect(normalizeModelList({ models: [{ bad: true }, fullModel] })).toEqual([fullModel]);
  });

  it('проставляет безопасные дефолты', () => {
    expect(normalizeModelList([{ name: 'tiny' }])).toEqual([
      {
        name: 'tiny',
        supportsTools: false,
        family: '',
        parameterSize: '',
        isCodeModel: false,
        suitableRoles: [],
        roleNotes: {},
      },
    ]);
  });
});
