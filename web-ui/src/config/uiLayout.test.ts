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

  it('открывает fileViewer из любого другого режима', () => {
    expect(toggleSystemPanelMode(SYSTEM_PANEL_MODES.rag, SYSTEM_PANEL_MODES.fileViewer)).toBe(SYSTEM_PANEL_MODES.fileViewer);
    expect(toggleSystemPanelMode(SYSTEM_PANEL_MODES.logs, SYSTEM_PANEL_MODES.fileViewer)).toBe(SYSTEM_PANEL_MODES.fileViewer);
    expect(toggleSystemPanelMode(null, SYSTEM_PANEL_MODES.fileViewer)).toBe(SYSTEM_PANEL_MODES.fileViewer);
  });

  it('закрывает fileViewer при повторном выборе', () => {
    expect(toggleSystemPanelMode(SYSTEM_PANEL_MODES.fileViewer, SYSTEM_PANEL_MODES.fileViewer)).toBeNull();
  });
});

describe('SYSTEM_PANEL_MODES', () => {
  it('содержит все 4 режима панели (rag, logs, settings, fileViewer)', () => {
    expect(SYSTEM_PANEL_MODES.rag).toBe('rag');
    expect(SYSTEM_PANEL_MODES.logs).toBe('logs');
    expect(SYSTEM_PANEL_MODES.settings).toBe('settings');
    expect(SYSTEM_PANEL_MODES.fileViewer).toBe('fileViewer');
    expect(Object.keys(SYSTEM_PANEL_MODES)).toHaveLength(4);
  });
});

describe('UI_LAYOUT', () => {
  it('содержит CSS clamp токены для адаптивной ширины панелей', () => {
    expect(UI_LAYOUT.sidebar.width).toContain('clamp');
    expect(UI_LAYOUT.systemPanel.width).toContain('clamp');
  });

  it('содержит параметры анимации collapse/expand сайдбара', () => {
    expect(UI_LAYOUT.sidebar.collapsedWidth).toBe('0px');
    expect(UI_LAYOUT.sidebar.transitionMs).toBeGreaterThan(0);
  });

  it('содержит параметры анимации системной панели', () => {
    expect(UI_LAYOUT.systemPanel.transitionMs).toBeGreaterThan(0);
  });
});
