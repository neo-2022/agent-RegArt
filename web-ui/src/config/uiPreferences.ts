/**
 * Настройки UI, сохраняемые между сессиями.
 *
 * Храним только безопасные UX-параметры интерфейса, без чувствительных данных.
 */
export interface UiPreferences {
  reducedMotion: boolean;
  compactSidebar: boolean;
  inferenceProfile: 'economy' | 'standard' | 'deep';
}

export const UI_PREFERENCES_STORAGE_KEY = 'web_ui_preferences';

export const DEFAULT_UI_PREFERENCES: UiPreferences = {
  reducedMotion: false,
  compactSidebar: false,
  inferenceProfile: 'standard',
};

export function parseUiPreferences(raw: string | null): UiPreferences {
  if (!raw) {
    return DEFAULT_UI_PREFERENCES;
  }

  try {
    const parsed = JSON.parse(raw) as Partial<UiPreferences>;
    return {
      reducedMotion: Boolean(parsed.reducedMotion),
      compactSidebar: Boolean(parsed.compactSidebar),
      inferenceProfile: parsed.inferenceProfile === 'economy' || parsed.inferenceProfile === 'deep'
        ? parsed.inferenceProfile
        : 'standard',
    };
  } catch {
    return DEFAULT_UI_PREFERENCES;
  }
}
