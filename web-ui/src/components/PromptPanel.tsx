/**
 * PromptPanel — inline-панель выбора и редактирования промптов.
 *
 * Согласно UI_UX_Design_Spec:
 * - Заменяет модальное окно (overlay) на встроенную панель
 * - Не перекрывает контент чата
 * - Плавная анимация раскрытия (slide-down + fade)
 * - Список файлов промптов с radio-выбором
 * - Inline-редактор для изменения текста промпта
 *
 * Edge-cases:
 * - Пустой список промптов — заглушка
 * - Длинные имена файлов — обрезка с ellipsis
 * - Ошибка загрузки — отображение сообщения об ошибке
 */
import { useState, useRef } from 'react';

export interface PromptFileItem {
  /** Имя файла промпта */
  fileName: string;
  /** Является ли файл текущим активным промптом */
  isActive: boolean;
}

interface PromptPanelProps {
  /** Видимость панели */
  isOpen: boolean;
  /** Callback закрытия панели */
  onClose: () => void;
  /** Имя агента, для которого выбирается промпт */
  agentName: string;
  /** Список файлов промптов */
  promptFiles: PromptFileItem[];
  /** Текст текущего промпта (для просмотра/редактирования) */
  promptText: string;
  /** Callback выбора файла промпта */
  onSelectPrompt: (fileName: string) => void;
  /** Callback сохранения изменённого промпта */
  onSavePrompt: (text: string) => void;
  /** Статус сохранения */
  saveStatus: 'idle' | 'saving' | 'success' | 'error';
  /** Сообщение об ошибке сохранения */
  saveError?: string;
}

/**
 * Inline-панель для управления промптами агента.
 * Встраивается непосредственно в карточку агента (не перекрывает контент).
 */
export function PromptPanel({
  isOpen,
  onClose,
  agentName,
  promptFiles,
  promptText,
  onSelectPrompt,
  onSavePrompt,
  saveStatus,
  saveError,
}: PromptPanelProps) {
  const [editMode, setEditMode] = useState(false);
  // Храним предыдущее значение promptText для сброса editText при смене промпта извне
  const [prevPromptText, setPrevPromptText] = useState(promptText);
  const [editText, setEditText] = useState(promptText);
  const panelRef = useRef<HTMLDivElement>(null);

  // Синхронизация: при смене promptText извне сбрасываем редактор
  if (promptText !== prevPromptText) {
    setPrevPromptText(promptText);
    setEditText(promptText);
    setEditMode(false);
  }

  if (!isOpen) return null;

  const handleSave = () => {
    onSavePrompt(editText);
  };

  const handleCancel = () => {
    setEditText(promptText);
    setEditMode(false);
  };

  return (
    <div
      className="prompt-panel"
      ref={panelRef}
      role="region"
      aria-label={`Промпт агента ${agentName}`}
    >
      <div className="prompt-panel-header">
        <span className="prompt-panel-title">Промпт: {agentName}</span>
        <button
          className="prompt-panel-close"
          onClick={onClose}
          type="button"
          aria-label="Закрыть панель промптов"
        >
          ×
        </button>
      </div>

      {/* Список файлов промптов */}
      <div className="prompt-panel-files">
        {promptFiles.length === 0 ? (
          <div className="prompt-panel-empty">Файлы промптов не найдены</div>
        ) : (
          promptFiles.map((file) => (
            <label
              key={file.fileName}
              className={`prompt-panel-file-item ${file.isActive ? 'active' : ''}`}
            >
              <input
                type="radio"
                name={`prompt-${agentName}`}
                checked={file.isActive}
                onChange={() => onSelectPrompt(file.fileName)}
              />
              <span className="prompt-panel-file-name">{file.fileName}</span>
            </label>
          ))
        )}
      </div>

      {/* Область просмотра/редактирования промпта */}
      <div className="prompt-panel-editor">
        {editMode ? (
          <>
            <textarea
              className="prompt-panel-textarea"
              value={editText}
              onChange={(e) => setEditText(e.target.value)}
              rows={6}
              aria-label="Текст промпта"
            />
            <div className="prompt-panel-actions">
              <button
                className="prompt-panel-btn prompt-panel-btn-save"
                onClick={handleSave}
                disabled={saveStatus === 'saving'}
                type="button"
              >
                {saveStatus === 'saving' ? 'Сохранение...' : 'Сохранить'}
              </button>
              <button
                className="prompt-panel-btn prompt-panel-btn-cancel"
                onClick={handleCancel}
                type="button"
              >
                Отмена
              </button>
            </div>
          </>
        ) : (
          <div className="prompt-panel-preview">
            <pre className="prompt-panel-text">{promptText || 'Промпт не загружен'}</pre>
            <button
              className="prompt-panel-btn prompt-panel-btn-edit"
              onClick={() => setEditMode(true)}
              type="button"
            >
              Редактировать
            </button>
          </div>
        )}
        {saveStatus === 'success' && (
          <div className="prompt-panel-status success">Промпт сохранён</div>
        )}
        {saveStatus === 'error' && (
          <div className="prompt-panel-status error">{saveError || 'Ошибка сохранения'}</div>
        )}
      </div>
    </div>
  );
}
