import { describe, expect, it } from 'vitest';

import { DEFAULT_UI_PREFERENCES, parseUiPreferences } from './uiPreferences';

describe('parseUiPreferences', () => {
  it('возвращает дефолты при пустом значении', () => {
    expect(parseUiPreferences(null)).toEqual(DEFAULT_UI_PREFERENCES);
  });

  it('возвращает дефолты при невалидном JSON', () => {
    expect(parseUiPreferences('{bad-json')).toEqual(DEFAULT_UI_PREFERENCES);
  });

  it('корректно извлекает boolean-настройки', () => {
    expect(parseUiPreferences(JSON.stringify({ reducedMotion: true, compactSidebar: true }))).toEqual({
      reducedMotion: true,
      compactSidebar: true,
    });
  });
});
