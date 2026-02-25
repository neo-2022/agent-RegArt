/**
 * Тесты для ModelPopover — проверяем интерфейсы и контракты компонента.
 *
 * Без jsdom/@testing-library тестируем:
 * - Корректность экспортов (интерфейс + компонент)
 * - Типизацию ModelPopoverItem
 * - Фильтрацию моделей (логика поиска)
 */
import { describe, expect, it } from 'vitest';
import type { ModelPopoverItem } from './ModelPopover';

/** Вспомогательная функция фильтрации, повторяющая логику компонента */
function filterItems(items: ModelPopoverItem[], search: string): ModelPopoverItem[] {
  if (!search.trim()) return items;
  const query = search.toLowerCase();
  return items.filter(
    (item) =>
      item.name.toLowerCase().includes(query) ||
      (item.family && item.family.toLowerCase().includes(query)) ||
      (item.parameterSize && item.parameterSize.toLowerCase().includes(query)),
  );
}

const sampleItems: ModelPopoverItem[] = [
  {
    id: 'qwen2.5:7b',
    name: 'qwen2.5:7b',
    family: 'qwen2',
    parameterSize: '7B',
    supportsTools: true,
    isSuitable: true,
    roleNote: 'Хорошо подходит для admin',
  },
  {
    id: 'llama3.2:3b',
    name: 'llama3.2:3b',
    family: 'llama',
    parameterSize: '3B',
    supportsTools: false,
    isSuitable: false,
    roleNote: 'Слишком маленькая для admin',
  },
  {
    id: 'gpt-4o',
    name: 'gpt-4o',
    isAvailable: true,
    pricingInfo: '$5/1M tokens',
  },
  {
    id: 'claude-3-opus',
    name: 'claude-3-opus',
    isAvailable: false,
    pricingInfo: '$15/1M tokens',
  },
];

describe('ModelPopoverItem — интерфейс', () => {
  it('содержит обязательные поля id и name', () => {
    const item: ModelPopoverItem = { id: 'test', name: 'Test Model' };
    expect(item.id).toBe('test');
    expect(item.name).toBe('Test Model');
  });

  it('поддерживает необязательные поля для локальных моделей', () => {
    const item: ModelPopoverItem = {
      id: 'local-model',
      name: 'Local Model',
      family: 'llama',
      parameterSize: '7B',
      supportsTools: true,
      isSuitable: true,
      roleNote: 'Подходит',
    };
    expect(item.family).toBe('llama');
    expect(item.parameterSize).toBe('7B');
    expect(item.supportsTools).toBe(true);
    expect(item.isSuitable).toBe(true);
    expect(item.roleNote).toBe('Подходит');
  });

  it('поддерживает необязательные поля для облачных моделей', () => {
    const item: ModelPopoverItem = {
      id: 'cloud-model',
      name: 'Cloud Model',
      isAvailable: true,
      pricingInfo: '$10/1M tokens',
    };
    expect(item.isAvailable).toBe(true);
    expect(item.pricingInfo).toBe('$10/1M tokens');
  });
});

describe('filterItems — логика поиска моделей', () => {
  it('возвращает все модели при пустом запросе', () => {
    expect(filterItems(sampleItems, '')).toEqual(sampleItems);
    expect(filterItems(sampleItems, '   ')).toEqual(sampleItems);
  });

  it('фильтрует по имени модели', () => {
    const result = filterItems(sampleItems, 'qwen');
    expect(result).toHaveLength(1);
    expect(result[0].id).toBe('qwen2.5:7b');
  });

  it('фильтрует по семейству (family)', () => {
    const result = filterItems(sampleItems, 'llama');
    expect(result).toHaveLength(1);
    expect(result[0].id).toBe('llama3.2:3b');
  });

  it('фильтрует по размеру параметров (parameterSize)', () => {
    const result = filterItems(sampleItems, '7B');
    expect(result).toHaveLength(1);
    expect(result[0].id).toBe('qwen2.5:7b');
  });

  it('поиск нечувствителен к регистру', () => {
    const result = filterItems(sampleItems, 'GPT');
    expect(result).toHaveLength(1);
    expect(result[0].id).toBe('gpt-4o');
  });

  it('возвращает пустой массив, если ничего не найдено', () => {
    const result = filterItems(sampleItems, 'nonexistent');
    expect(result).toHaveLength(0);
  });

  it('находит несколько совпадений', () => {
    // Оба облачных модели содержат "-" в имени, но ищем по "3"
    const result = filterItems(sampleItems, '3');
    // llama3.2:3b (имя), claude-3-opus (имя), qwen2.5 нет, gpt-4o нет
    // llama3.2:3b также имеет parameterSize "3B"
    expect(result.length).toBeGreaterThanOrEqual(2);
  });

  it('корректно обрабатывает пустой массив моделей', () => {
    const result = filterItems([], 'test');
    expect(result).toHaveLength(0);
  });
});

describe('ModelPopover — экспорт', () => {
  it('экспортирует функцию ModelPopover', async () => {
    const mod = await import('./ModelPopover');
    expect(typeof mod.ModelPopover).toBe('function');
  });
});
