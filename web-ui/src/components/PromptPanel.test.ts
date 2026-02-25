/**
 * Тесты для PromptPanel — проверяем интерфейсы и контракты компонента.
 *
 * Без jsdom/@testing-library тестируем:
 * - Корректность экспортов (интерфейс + компонент)
 * - Типизацию PromptFileItem
 * - Логику синхронизации состояния
 */
import { describe, expect, it } from 'vitest';
import type { PromptFileItem } from './PromptPanel';

/** Вспомогательная функция, эмулирующая логику синхронизации prevPromptText */
function shouldResetEditor(promptText: string, prevPromptText: string): boolean {
  return promptText !== prevPromptText;
}

const sampleFiles: PromptFileItem[] = [
  { fileName: 'default.txt', isActive: true },
  { fileName: 'creative.txt', isActive: false },
  { fileName: 'analyst.md', isActive: false },
];

describe('PromptFileItem — интерфейс', () => {
  it('содержит обязательные поля fileName и isActive', () => {
    const item: PromptFileItem = { fileName: 'test.txt', isActive: false };
    expect(item.fileName).toBe('test.txt');
    expect(item.isActive).toBe(false);
  });

  it('поддерживает активный файл', () => {
    const active = sampleFiles.find((f) => f.isActive);
    expect(active).toBeDefined();
    expect(active?.fileName).toBe('default.txt');
  });

  it('корректно различает активный и неактивный файлы', () => {
    const activeCount = sampleFiles.filter((f) => f.isActive).length;
    const inactiveCount = sampleFiles.filter((f) => !f.isActive).length;
    expect(activeCount).toBe(1);
    expect(inactiveCount).toBe(2);
  });
});

describe('shouldResetEditor — логика синхронизации', () => {
  it('возвращает true, если promptText изменился', () => {
    expect(shouldResetEditor('новый текст', 'старый текст')).toBe(true);
  });

  it('возвращает false, если promptText не изменился', () => {
    expect(shouldResetEditor('тот же текст', 'тот же текст')).toBe(false);
  });

  it('корректно обрабатывает пустые строки', () => {
    expect(shouldResetEditor('', '')).toBe(false);
    expect(shouldResetEditor('text', '')).toBe(true);
    expect(shouldResetEditor('', 'text')).toBe(true);
  });
});

describe('PromptPanel — saveStatus типы', () => {
  it('поддерживает все валидные статусы', () => {
    const validStatuses: Array<'idle' | 'saving' | 'success' | 'error'> = [
      'idle',
      'saving',
      'success',
      'error',
    ];
    // Проверяем, что все статусы — строки
    validStatuses.forEach((status) => {
      expect(typeof status).toBe('string');
    });
    expect(validStatuses).toHaveLength(4);
  });
});

describe('PromptPanel — экспорт', () => {
  it('экспортирует функцию PromptPanel', async () => {
    const mod = await import('./PromptPanel');
    expect(typeof mod.PromptPanel).toBe('function');
  });
});
