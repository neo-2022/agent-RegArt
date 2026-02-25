import { describe, expect, it } from 'vitest';

import { normalizeWorkspaceList } from './workspaceApi';

describe('normalizeWorkspaceList', () => {
  const sample = { ID: 1, Name: 'Main', Path: '/tmp/main' };

  it('поддерживает ответ в виде массива', () => {
    expect(normalizeWorkspaceList([sample])).toEqual([sample]);
  });

  it('поддерживает объект-обёртку с ключом workspaces', () => {
    expect(normalizeWorkspaceList({ workspaces: [sample] })).toEqual([sample]);
  });

  it('поддерживает объект-обёртку с ключом data', () => {
    expect(normalizeWorkspaceList({ data: [sample] })).toEqual([sample]);
  });

  it('фильтрует невалидные элементы и нормализует строковый ID', () => {
    expect(normalizeWorkspaceList({ items: [{ ID: '7', Name: 'W', Path: '/w' }, { bad: true }] })).toEqual([
      { ID: 7, Name: 'W', Path: '/w' },
    ]);
  });

  it('возвращает пустой массив для невалидной формы', () => {
    expect(normalizeWorkspaceList('bad-shape')).toEqual([]);
  });
});
