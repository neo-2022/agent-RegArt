/**
 * ModelPopover — premium popover для выбора модели вместо стандартного <select>.
 *
 * Согласно UI_UX_Design_Spec:
 * - Каждая модель отображается как карточка с описанием
 * - Поддержка поиска по моделям
 * - Мягкий hover highlight
 * - Плавная анимация (scale + fade)
 * - Закрытие по клику вне, Escape, выбору модели
 *
 * Edge-cases:
 * - Пустой список моделей — показываем заглушку
 * - Длинные имена — обрезаются с ellipsis
 * - Автофокус на поле поиска при открытии
 */
import React, { useState, useRef, useEffect, useCallback } from 'react';

/** Описание модели для отображения в popover. */
export interface ModelPopoverItem {
  /** Идентификатор модели (передаётся при выборе) */
  id: string;
  /** Отображаемое имя модели */
  name: string;
  /** Семейство модели (например, llama, gpt) */
  family?: string;
  /** Размер параметров (например, 8B, 70B) */
  parameterSize?: string;
  /** Поддерживает ли модель вызов инструментов */
  supportsTools?: boolean;
  /** Подходит ли модель для текущей роли */
  isSuitable?: boolean;
  /** Примечание о пригодности для роли */
  roleNote?: string;
  /** Доступность (для облачных моделей) */
  isAvailable?: boolean;
  /** Информация о ценах (для облачных моделей) */
  pricingInfo?: string;
}

interface ModelPopoverProps {
  /** Список моделей для отображения */
  items: ModelPopoverItem[];
  /** Текущая выбранная модель */
  selectedId: string;
  /** Callback при выборе модели */
  onSelect: (modelId: string) => void;
  /** Текст плейсхолдера, если ни одна модель не выбрана */
  placeholder?: string;
  /** Провайдер (для отображения контекста) */
  provider?: string;
}

/**
 * Premium popover для выбора модели.
 * Заменяет стандартный <select> на кастомный UI с поиском и карточками.
 */
export function ModelPopover({
  items,
  selectedId,
  onSelect,
  placeholder = 'Выберите модель',
  provider,
}: ModelPopoverProps) {
  const [isOpen, setIsOpen] = useState(false);
  const [search, setSearch] = useState('');
  const containerRef = useRef<HTMLDivElement>(null);
  const searchInputRef = useRef<HTMLInputElement>(null);

  // Закрытие popover по клику вне контейнера
  const handleClickOutside = useCallback((event: MouseEvent) => {
    if (containerRef.current && !containerRef.current.contains(event.target as Node)) {
      setIsOpen(false);
      setSearch('');
    }
  }, []);

  // Закрытие по Escape
  const handleKeyDown = useCallback((event: KeyboardEvent) => {
    if (event.key === 'Escape') {
      setIsOpen(false);
      setSearch('');
    }
  }, []);

  useEffect(() => {
    if (isOpen) {
      document.addEventListener('mousedown', handleClickOutside);
      document.addEventListener('keydown', handleKeyDown);
      // Автофокус на поле поиска при открытии (с небольшой задержкой для анимации)
      requestAnimationFrame(() => searchInputRef.current?.focus());
    }
    return () => {
      document.removeEventListener('mousedown', handleClickOutside);
      document.removeEventListener('keydown', handleKeyDown);
    };
  }, [isOpen, handleClickOutside, handleKeyDown]);

  // Фильтрация моделей по поисковому запросу
  const filteredItems = items.filter((item) => {
    if (!search.trim()) return true;
    const query = search.toLowerCase();
    return (
      item.name.toLowerCase().includes(query) ||
      (item.family && item.family.toLowerCase().includes(query)) ||
      (item.parameterSize && item.parameterSize.toLowerCase().includes(query))
    );
  });

  const selectedItem = items.find((item) => item.id === selectedId);

  const handleSelect = (modelId: string) => {
    onSelect(modelId);
    setIsOpen(false);
    setSearch('');
  };

  const togglePopover = (e: React.MouseEvent) => {
    e.stopPropagation();
    setIsOpen((prev) => !prev);
    if (isOpen) setSearch('');
  };

  return (
    <div className="model-popover-container" ref={containerRef}>
      {/* Кнопка-триггер: отображает текущую модель или плейсхолдер */}
      <button
        className={`model-popover-trigger ${isOpen ? 'open' : ''}`}
        onClick={togglePopover}
        type="button"
        aria-haspopup="listbox"
        aria-expanded={isOpen}
        title={selectedItem ? selectedItem.name : placeholder}
      >
        <span className="model-popover-trigger-text">
          {selectedItem ? selectedItem.name : placeholder}
        </span>
        <span className={`model-popover-chevron ${isOpen ? 'open' : ''}`}>▾</span>
      </button>

      {/* Выпадающий popover */}
      {isOpen && (
        <div className="model-popover-dropdown" role="listbox">
          {/* Поле поиска */}
          <div className="model-popover-search-wrapper">
            <input
              ref={searchInputRef}
              className="model-popover-search"
              type="text"
              placeholder="Поиск модели..."
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              aria-label="Поиск модели"
            />
          </div>

          {/* Список моделей */}
          <div className="model-popover-list">
            {filteredItems.length === 0 ? (
              <div className="model-popover-empty">
                {items.length === 0
                  ? (provider === 'lmstudio' ? 'Нет моделей — нажмите ↻' : 'Нет доступных моделей')
                  : 'Ничего не найдено'}
              </div>
            ) : (
              filteredItems.map((item) => (
                <button
                  key={item.id}
                  className={`model-popover-item ${item.id === selectedId ? 'selected' : ''} ${item.isSuitable === false ? 'unsuitable' : ''}`}
                  onClick={() => handleSelect(item.id)}
                  role="option"
                  aria-selected={item.id === selectedId}
                  type="button"
                >
                  <div className="model-popover-item-main">
                    <span className="model-popover-item-icon">
                      {item.isSuitable === false ? '✗' : item.isAvailable === false ? '○' : '✓'}
                    </span>
                    <span className="model-popover-item-name">{item.name}</span>
                  </div>
                  <div className="model-popover-item-meta">
                    {item.family && (
                      <span className="model-popover-item-family">{item.family}</span>
                    )}
                    {item.parameterSize && (
                      <span className="model-popover-item-size">{item.parameterSize}</span>
                    )}
                    {item.pricingInfo && (
                      <span className="model-popover-item-price">{item.pricingInfo}</span>
                    )}
                    {item.supportsTools && (
                      <span className="model-popover-item-tools" title="Поддерживает инструменты">🔧</span>
                    )}
                  </div>
                  {item.roleNote && (
                    <div className={`model-popover-item-note ${item.isSuitable ? 'suitable' : 'unsuitable'}`}>
                      {item.roleNote}
                    </div>
                  )}
                </button>
              ))
            )}
          </div>
        </div>
      )}
    </div>
  );
}
