import { describe, expect, it } from 'vitest';
import { LOG_LEVEL_OPTIONS, LOG_LEVEL_VALUES, LOG_SERVICE_OPTIONS, LOG_SERVICE_VALUES } from './logFilters';

describe('logFilters', () => {
  it('содержит обязательные уровни логов с русскими подписями', () => {
    expect(LOG_LEVEL_OPTIONS).toEqual([
      { value: LOG_LEVEL_VALUES.all, label: 'Все уровни' },
      { value: LOG_LEVEL_VALUES.error, label: 'Ошибка' },
      { value: LOG_LEVEL_VALUES.warn, label: 'Предупреждение' },
      { value: LOG_LEVEL_VALUES.info, label: 'Информация' },
    ]);
  });

  it('содержит все ожидаемые сервисы для фильтрации', () => {
    expect(LOG_SERVICE_OPTIONS).toEqual([
      { value: LOG_SERVICE_VALUES.all, label: 'Все сервисы' },
      { value: LOG_SERVICE_VALUES.agent, label: 'Агент' },
      { value: LOG_SERVICE_VALUES.tools, label: 'Инструменты' },
      { value: LOG_SERVICE_VALUES.memory, label: 'Память' },
      { value: LOG_SERVICE_VALUES.gateway, label: 'Шлюз API' },
    ]);
  });
});
