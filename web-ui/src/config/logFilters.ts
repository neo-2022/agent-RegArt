/**
 * Централизованные опции фильтров системных логов.
 *
 * Зачем:
 * - убираем "магические" строки из JSX;
 * - обеспечиваем единый источник правды для значений фильтров и русских подписей;
 * - упрощаем расширение списка уровней/сервисов и покрытие unit-тестами.
 */
export const LOG_LEVEL_VALUES = {
  all: 'all',
  error: 'error',
  warn: 'warn',
  info: 'info',
} as const;

export type LogLevelValue = (typeof LOG_LEVEL_VALUES)[keyof typeof LOG_LEVEL_VALUES];

export const LOG_LEVEL_OPTIONS: ReadonlyArray<{ value: LogLevelValue; label: string }> = [
  { value: LOG_LEVEL_VALUES.all, label: 'Все уровни' },
  { value: LOG_LEVEL_VALUES.error, label: 'Ошибка' },
  { value: LOG_LEVEL_VALUES.warn, label: 'Предупреждение' },
  { value: LOG_LEVEL_VALUES.info, label: 'Информация' },
] as const;

export const LOG_SERVICE_VALUES = {
  all: 'all',
  agent: 'agent-service',
  tools: 'tools-service',
  memory: 'memory-service',
  gateway: 'api-gateway',
} as const;

export type LogServiceValue = (typeof LOG_SERVICE_VALUES)[keyof typeof LOG_SERVICE_VALUES];

export const LOG_SERVICE_OPTIONS: ReadonlyArray<{ value: LogServiceValue; label: string }> = [
  { value: LOG_SERVICE_VALUES.all, label: 'Все сервисы' },
  { value: LOG_SERVICE_VALUES.agent, label: 'Агент' },
  { value: LOG_SERVICE_VALUES.tools, label: 'Инструменты' },
  { value: LOG_SERVICE_VALUES.memory, label: 'Память' },
  { value: LOG_SERVICE_VALUES.gateway, label: 'Шлюз API' },
] as const;
