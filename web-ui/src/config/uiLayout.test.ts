import { describe, expect, it } from 'vitest';

import { SYSTEM_PANEL_MODES, toggleSystemPanelMode, UI_LAYOUT } from './uiLayout';

describe('toggleSystemPanelMode', () => {
  it('открывает панель при выборе режима из закрытого состояния', () => {
    expect(toggleSystemPanelMode(null, SYSTEM_PANEL_MODES.rag)).toBe(SYSTEM_PANEL_MODES.rag);
  });

  it('закрывает панель при повторном выборе того же режима', () => {
    expect(toggleSystemPanelMode(SYSTEM_PANEL_MODES.logs, SYSTEM_PANEL_MODES.logs)).toBeNull();
  });

  it('переключает контент внутри правой панели без промежуточного null', () => {
    expect(toggleSystemPanelMode(SYSTEM_PANEL_MODES.rag, SYSTEM_PANEL_MODES.logs)).toBe(SYSTEM_PANEL_MODES.logs);
  });
});

describe('UI_LAYOUT', () => {
  it('содержит CSS clamp токены для адаптивной ширины панелей', () => {
    expect(UI_LAYOUT.sidebar.width).toContain('clamp');
    expect(UI_LAYOUT.systemPanel.width).toContain('clamp');
  });
});
