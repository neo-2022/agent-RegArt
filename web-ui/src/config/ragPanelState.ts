/**
 * Формальная модель состояний RAG-панели согласно дизайн/UX спецификации.
 *
 * Почему отдельный модуль:
 * - состояние должно вычисляться детерминированно и тестироваться отдельно от React-компонента;
 * - в дальнейшем сюда можно подключить серверные сигналы (например, конфликт версий/индексации).
 */
export const RAG_PANEL_STATES = {
  empty: 'empty',
  processing: 'processing',
  ready: 'ready',
  error: 'error',
  outdated: 'outdated',
  conflict: 'conflict',
} as const;

export type RagPanelState = (typeof RAG_PANEL_STATES)[keyof typeof RAG_PANEL_STATES];

export interface RagPanelStateInput {
  isUploading: boolean;
  hasError: boolean;
  hasAnyFiles: boolean;
  statsFilesCount: number;
  loadedFilesCount: number;
  hasNameConflict: boolean;
}

/**
 * Приоритеты состояния (от более критичного к менее критичному):
 * 1) error
 * 2) processing
 * 3) conflict
 * 4) outdated
 * 5) ready
 * 6) empty
 */
export function deriveRagPanelState(input: RagPanelStateInput): RagPanelState {
  if (input.hasError) {
    return RAG_PANEL_STATES.error;
  }
  if (input.isUploading) {
    return RAG_PANEL_STATES.processing;
  }
  if (input.hasNameConflict) {
    return RAG_PANEL_STATES.conflict;
  }
  if (input.statsFilesCount > 0 && input.loadedFilesCount !== input.statsFilesCount) {
    return RAG_PANEL_STATES.outdated;
  }
  if (input.hasAnyFiles) {
    return RAG_PANEL_STATES.ready;
  }
  return RAG_PANEL_STATES.empty;
}

export const RAG_PANEL_STATE_LABELS: Record<RagPanelState, string> = {
  [RAG_PANEL_STATES.empty]: 'Пусто: база знаний ещё не содержит файлов.',
  [RAG_PANEL_STATES.processing]: 'Обработка: файлы загружаются и индексируются.',
  [RAG_PANEL_STATES.ready]: 'Готово: база знаний готова к использованию.',
  [RAG_PANEL_STATES.error]: 'Ошибка: не удалось обновить данные RAG.',
  [RAG_PANEL_STATES.outdated]: 'Устарело: данные панели не синхронизированы со статистикой.',
  [RAG_PANEL_STATES.conflict]: 'Конфликт: обнаружены одинаковые имена файлов в разных папках.',
};
