/**
 * Централизованные UI-константы для layout чата.
 *
 * Зачем:
 * - Убираем магические значения ширины панелей и длительности анимаций из компонентов/стилей.
 * - Даём единый источник правды для тестов и последующих итераций UX-рефакторинга.
 */
export const UI_LAYOUT = {
  sidebar: {
    width: 'clamp(16rem, 20vw, 19rem)',
    compactWidth: '12.5rem',
  },
  systemPanel: {
    width: 'clamp(20rem, 28vw, 28rem)',
    transitionMs: 280,
  },
} as const;

/**
 * Режимы правой системной панели.
 *
 * Важно: в design spec панель должна переключать режимы внутри контейнера,
 * а не открывать отдельные перекрывающие оверлеи.
 */
export const SYSTEM_PANEL_MODES = {
  rag: 'rag',
  logs: 'logs',
  settings: 'settings',
} as const;

export type SystemPanelMode = (typeof SYSTEM_PANEL_MODES)[keyof typeof SYSTEM_PANEL_MODES];

/**
 * Переключает правую панель между режимами.
 *
 * Поведение:
 * - Повторный клик по активному режиму закрывает панель (null).
 * - Клик по другому режиму переключает контент без промежуточного оверлея.
 */
export function toggleSystemPanelMode(
  currentMode: SystemPanelMode | null,
  nextMode: SystemPanelMode,
): SystemPanelMode | null {
  return currentMode === nextMode ? null : nextMode;
}
