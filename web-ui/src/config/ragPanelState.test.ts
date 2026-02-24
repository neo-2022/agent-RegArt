import { describe, expect, it } from 'vitest';

import { deriveRagPanelState, RAG_PANEL_STATES } from './ragPanelState';

const baseInput = {
  isUploading: false,
  hasError: false,
  hasAnyFiles: false,
  statsFilesCount: 0,
  loadedFilesCount: 0,
  hasNameConflict: false,
};

describe('deriveRagPanelState', () => {
  it('возвращает error при ошибке загрузки независимо от остальных признаков', () => {
    expect(deriveRagPanelState({ ...baseInput, hasError: true, isUploading: true })).toBe(RAG_PANEL_STATES.error);
  });

  it('возвращает processing во время загрузки', () => {
    expect(deriveRagPanelState({ ...baseInput, isUploading: true })).toBe(RAG_PANEL_STATES.processing);
  });

  it('возвращает conflict при конфликте имён', () => {
    expect(deriveRagPanelState({ ...baseInput, hasAnyFiles: true, hasNameConflict: true })).toBe(RAG_PANEL_STATES.conflict);
  });

  it('возвращает outdated при рассинхроне counts', () => {
    expect(deriveRagPanelState({ ...baseInput, hasAnyFiles: true, statsFilesCount: 5, loadedFilesCount: 4 })).toBe(RAG_PANEL_STATES.outdated);
  });

  it('возвращает ready когда данные синхронизированы и файлы есть', () => {
    expect(deriveRagPanelState({ ...baseInput, hasAnyFiles: true, statsFilesCount: 3, loadedFilesCount: 3 })).toBe(RAG_PANEL_STATES.ready);
  });

  it('возвращает empty при отсутствии файлов', () => {
    expect(deriveRagPanelState(baseInput)).toBe(RAG_PANEL_STATES.empty);
  });
});
