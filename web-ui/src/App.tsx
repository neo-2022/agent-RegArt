// Главный компонент веб-интерфейса Agent Core NG.
// Реализует полнофункциональный чат с AI-агентами (Admin, Coder, Novice),
// управление моделями (локальные Ollama + облачные OpenAI/Anthropic/YandexGPT/GigaChat),
// рабочие пространства (Workspaces), прикрепление файлов, голосовой ввод,
// межагентные обсуждения и RAG-поиск по базе знаний.
//
// Архитектура:
//   - Все API-запросы идут через API Gateway (порт 8080)
//   - Состояние управляется через React useState hooks
//   - Markdown-рендеринг через react-markdown + react-syntax-highlighter
//   - Голосовой ввод через Web Speech API (ru-RU)
//   - Прикрепление файлов через FileReader API
import React, { useState, useEffect, useRef } from 'react';
import axios from 'axios';
import ReactMarkdown from 'react-markdown';
import { Prism as SyntaxHighlighter } from 'react-syntax-highlighter';
import { vscDarkPlus } from 'react-syntax-highlighter/dist/esm/styles/prism';
import './styles/App.css';

// AttachedFile — интерфейс прикреплённого файла.
// Содержит имя файла и его текстовое содержимое (прочитанное через FileReader).
interface AttachedFile {
  name: string;
  content: string;
}

// Message — интерфейс сообщения в чате.
// Роли: user (пользователь), assistant (ответ агента), system (системное уведомление).
// Поле agent указывает, какой агент ответил (для отображения правильного аватара).
// Поле files содержит прикреплённые файлы (опционально).
interface Message {
  role: 'user' | 'assistant' | 'system';
  content: string;
  agent?: string;
  files?: AttachedFile[];
  model?: string;
  sources?: Source[];
  timestamp?: string;
}

interface Source {
  title: string;
  content: string;
  score: number;
}

// Agent — интерфейс агента, полученный от бэкенда (/agents).
// Содержит имя, текущую модель, провайдера, поддержку инструментов, аватар и промпт.
interface Agent {
  name: string;
  model: string;
  provider: string;
  supportsTools: boolean;
  avatar: string;
  prompt_file?: string;
  prompt: string;
}

// Chat — интерфейс чата в боковой панели.
// Каждый чат имеет уникальный ID, имя, массив сообщений, превью последнего сообщения
// и флаг закрепления (pinned — закреплённые чаты отображаются сверху).
interface Chat {
  id: string;
  name: string;
  messages: Message[];
  lastMessage?: string;
  pinned: boolean;
}

// ModelInfo — информация о локальной модели Ollama.
// Включает автоматически определённые характеристики: семейство, размер, специализация,
// подходящие роли агентов и пояснения к каждой роли.
// Вся информация определяется динамически — никаких жёстких привязок в коде.
interface ModelInfo {
  name: string;
  supportsTools: boolean;
  family: string;
  parameterSize: string;
  isCodeModel: boolean;
  suitableRoles: string[];
  roleNotes: { [role: string]: string };
}

// ModelDetailInfo — детальная информация о модели провайдера (доступность, цена, активация).
// Приходит из бэкенда в поле models_detail ответа /providers.
// is_available=true — модель доступна прямо сейчас (яркая в UI)
// is_available=false — модель нельзя использовать, нужно активировать (тусклая в UI)
interface ModelDetailInfo {
  id: string;
  is_available: boolean;
  pricing_info: string;
  activation_hint: string;
}

interface SystemLog {
  ID: number;
  Level: string;
  Service: string;
  Message: string;
  Details: string;
  Resolved: boolean;
  CreatedAt: string;
}

// ProviderGuideInfo — подробное руководство по провайдеру.
// Содержит инструкции: как подключить, как выбрать модель, где оплатить, как проверить баланс.
interface ProviderGuideInfo {
  how_to_connect: string;
  how_to_choose: string;
  how_to_pay: string;
  how_to_balance: string;
}

// ProviderInfo — информация об облачном LLM-провайдере.
// hasKey указывает, настроен ли API-ключ для этого провайдера.
// models — список доступных моделей у провайдера.
// models_detail — детальная информация с ценами и подсказками по активации.
// guide — подробное руководство по подключению, оплате и проверке баланса.
interface ProviderInfo {
  name: string;
  enabled: boolean;
  models: string[];
  models_detail?: ModelDetailInfo[];
  hasKey: boolean;
  guide?: ProviderGuideInfo;
}

// WorkspaceInfo — информация о рабочем пространстве.
// Каждое пространство привязано к директории на ПК и имеет отдельную историю чатов.
interface WorkspaceInfo {
  ID: number;
  Name: string;
  Path: string;
}

// BUILT_IN_AVATARS — встроенные аватарки агентов (статические файлы в public/avatars/).
// Админ — византийский крест (символ власти и управления).
// Кодер — хакер с ноутбуком (программист).
// Послушник — силуэт человека (новичок).
const BUILT_IN_AVATARS: Record<string, string> = {
  admin: '/avatars/admin.jpg',
  coder: '/avatars/coder.jpg',
  novice: '/avatars/novice.jpg',
};

// DEFAULT_AGENTS — агенты по умолчанию, отображаемые когда бэкенд недоступен.
// Обеспечивает отображение аватарок и возможность переключения даже без подключения к серверу.
const DEFAULT_AGENTS: Agent[] = [
  { name: 'admin', model: '', provider: 'ollama', supportsTools: true, avatar: '', prompt: '' },
  { name: 'coder', model: '', provider: 'ollama', supportsTools: true, avatar: '', prompt: '' },
  { name: 'novice', model: '', provider: 'ollama', supportsTools: false, avatar: '', prompt: '' },
];

// CHUNK_SIZE — максимальный размер содержимого файла для отправки в одном сообщении.
// Файлы больше этого лимита обрезаются с предложением «продолжить чтение».
// 16000 символов ≈ 4000 токенов — безопасный размер для большинства моделей.
const CHUNK_SIZE = 16000;

// URL-адреса API — все запросы идут через API Gateway.
// GATEWAY_URL берётся из переменной окружения VITE_API_GATEWAY_URL.
// Если не задан — используется пустая строка (запросы на тот же хост).
const GATEWAY_URL = import.meta.env.VITE_API_GATEWAY_URL || '';
const API_BASE = `${GATEWAY_URL}/agents/`;        // Список агентов и чат
const MODELS_API = `${GATEWAY_URL}/models`;         // Локальные модели Ollama
const UPDATE_MODEL_API = `${GATEWAY_URL}/update-model`; // Смена модели агента
const AVATAR_UPLOAD_API = `${GATEWAY_URL}/avatar`;  // Загрузка аватара
const AVATAR_BASE = `${GATEWAY_URL}/uploads/avatars/`; // Базовый URL для аватаров
const PROMPTS_API = `${GATEWAY_URL}/prompts`;       // Файлы промптов
const LOAD_PROMPT_API = `${GATEWAY_URL}/prompts/load`; // Загрузка промпта из файла
const MEMORY_API = `${GATEWAY_URL}/memory`;         // RAG-поиск по базе знаний (memory-service)
const RAG_API = `${GATEWAY_URL}/rag`;              // RAG-эндпоинты agent-service
const PROVIDERS_API = `${GATEWAY_URL}/providers`;   // Облачные LLM-провайдеры

const WORKSPACES_API = `${GATEWAY_URL}/workspaces`; // Рабочие пространства
const UPDATE_PROMPT_API = `${GATEWAY_URL}/agent/prompt`; // Обновление промпта вручную
const LOGS_API = `${GATEWAY_URL}/logs`;               // Системные логи

// nameMap — словарь синонимов имён агентов для распознавания обращений.
// Поддерживает русские и английские варианты, включая опечатки ('колер' → coder).
// Используется в extractAgentNames() для определения, к какому агенту обращается пользователь.
const nameMap: { [key: string]: string } = {
  'админ': 'admin',
  'admin': 'admin',
  'администратор': 'admin',
  'кодер': 'coder',
  'колер': 'coder',
  'coder': 'coder',
  'программист': 'coder',
  'послушник': 'novice',
  'novice': 'novice',
  'новичок': 'novice',
};

// App — главный React-компонент приложения.
// Управляет всем состоянием: чаты, агенты, модели, провайдеры, пространства,
// прикреплённые файлы, голосовой ввод, RAG-режим.
function App() {
  // === Основное состояние чата ===
  // Загружаем историю чата из localStorage
  let initialMessages: Message[] = [];
  try {
    const savedMessages = localStorage.getItem('chat_messages');
    initialMessages = savedMessages ? JSON.parse(savedMessages) : [];
  } catch (e) {
    console.error('Failed to load messages from localStorage', e);
  }
  const [messages, setMessages] = useState<Message[]>(initialMessages);
  
  // Загружаем выбранного агента
  let savedAgent = 'admin';
  try {
    savedAgent = localStorage.getItem('current_agent') || 'admin';
  } catch (e) {
    console.error('Failed to load agent from localStorage', e);
  }
  const [currentAgent, setCurrentAgent] = useState(savedAgent);
  
  // Сохраняем сообщения в localStorage при изменении
  useEffect(() => {
    localStorage.setItem('chat_messages', JSON.stringify(messages));
  }, [messages]);
  
  // Сохраняем агента
  useEffect(() => {
    localStorage.setItem('current_agent', currentAgent);
  }, [currentAgent]);
  const [input, setInput] = useState('');                             // Текст в поле ввода
  const [agents, setAgents] = useState<Agent[]>([]);                  // Список агентов из бэкенда
  const [loading, setLoading] = useState(false);                      // Индикатор загрузки ответа
  const [chats, setChats] = useState<Chat[]>([
    { id: '1', name: 'Основной чат', messages: [], pinned: false },
    { id: '2', name: 'Второй чат', messages: [], pinned: false }
  ]);
  const [currentChatId, setCurrentChatId] = useState('1');            // ID активного чата
  const [models, setModels] = useState<ModelInfo[]>([]);              // Локальные модели Ollama

  // === UI-состояние ===
  const [menuChatId, setMenuChatId] = useState<string | null>(null);  // Открытое контекстное меню чата
  const [uploadingAvatar, setUploadingAvatar] = useState<string | null>(null); // Загрузка аватара
  const [showPromptModal, setShowPromptModal] = useState(false);      // Модальное окно промптов
  const [modalAgent, setModalAgent] = useState<string>('');           // Агент в модальном окне
  const [availablePrompts, setAvailablePrompts] = useState<string[]>([]); // Доступные файлы промптов
  const [selectedPrompt, setSelectedPrompt] = useState<string>('');   // Выбранный промпт
  const [promptText, setPromptText] = useState<string>('');           // Текст промпта для редактирования
  const [promptTab, setPromptTab] = useState<'edit' | 'files'>('edit'); // Вкладка в модальном окне промптов
  const [_ragFactText, _setRagFactText] = useState<string>('');         // Текст для добавления факта в RAG (зарезервировано)
  const [ragStats, setRagStats] = useState<{facts_count: number; files_count: number} | null>(null); // Статистика RAG
  const [agentsPanelOpen, setAgentsPanelOpen] = useState(true);       // Панель моделей открыта/закрыта
  const [speakingAgent, setSpeakingAgent] = useState<string | null>(null); // Говорящий агент (пульсация)
  const [ragEnabled, setRagEnabled] = useState(false);                // RAG-режим вкл/выкл
  const [showRagPanel, setShowRagPanel] = useState(false);            // RAG-панель открыта/закрыта
  const [attachedFiles, setAttachedFiles] = useState<AttachedFile[]>([]); // Прикреплённые файлы
  const [isListening, setIsListening] = useState(false);              // Голосовой ввод активен

  // === Облачные провайдеры и пространства ===
  const [modelMode, setModelMode] = useState<'local' | 'cloud'>('local'); // Режим: локальная/облачная
  const [selectedProvider, setSelectedProvider] = useState<string>('ollama'); // Выбранный провайдер
  const [providers, setProviders] = useState<ProviderInfo[]>([]);     // Список провайдеров
  const [cloudModels, setCloudModels] = useState<{[provider: string]: string[]}>({});  // Модели по провайдерам
  const [cloudModelsDetail, setCloudModelsDetail] = useState<{[provider: string]: ModelDetailInfo[]}>({});  // Детальная информация о моделях (цены, бесплатность)
  const [workspaces, setWorkspaces] = useState<WorkspaceInfo[]>([]);  // Рабочие пространства
  const [currentWorkspaceId, setCurrentWorkspaceId] = useState<number | null>(null);   // Активное пространство
  const [providerForm, setProviderForm] = useState<{api_key: string; base_url: string; folder_id: string; scope: string; service_account_json: string}>({api_key: '', base_url: '', folder_id: '', scope: '', service_account_json: ''});
  const [providerSaving, setProviderSaving] = useState(false);          // Индикатор сохранения провайдера
  const [providerError, setProviderError] = useState<string | null>(null); // Ошибка провайдера
  const [providerHint, setProviderHint] = useState<string | null>(null);   // Подсказка провайдера
  const [providerSuccess, setProviderSuccess] = useState<string | null>(null); // Успех провайдера
  const [providerGuideOpen, setProviderGuideOpen] = useState<string | null>(null); // Открытая справка провайдера
  const [guideTab, setGuideTab] = useState<'connect' | 'choose' | 'pay' | 'balance'>('connect'); // Вкладка справки
  const [refreshingProviders, setRefreshingProviders] = useState(false); // Индикатор обновления провайдеров
  const [ragUploadStatus, setRagUploadStatus] = useState<'idle' | 'uploading' | 'success' | 'error'>('idle');
  const [ragUploadMessage, setRagUploadMessage] = useState('');

  // === Дополнительное состояние для RAG-файлов и просмотра ===
  const [ragFiles, setRagFiles] = useState<{file_name: string; chunks_count: number}[]>([]);
  const [viewingFile, setViewingFile] = useState<AttachedFile | null>(null);
  const [promptSaveStatus, setPromptSaveStatus] = useState<'idle' | 'saving' | 'success' | 'error'>('idle');
  const [promptSaveError, setPromptSaveError] = useState('');

  const [showLogsPanel, setShowLogsPanel] = useState(false);
  const [systemLogs, setSystemLogs] = useState<SystemLog[]>([]);
  const [logLevelFilter, setLogLevelFilter] = useState<string>('all');
  const [logServiceFilter, setLogServiceFilter] = useState<string>('all');
  const [logsLoading, setLogsLoading] = useState(false);

  // === Рефы для DOM-элементов ===
  const recognitionRef = useRef<SpeechRecognition | null>(null);      // Web Speech API экземпляр
  const messagesEndRef = useRef<HTMLDivElement>(null);                // Якорь для автоскролла
  const fileInputRef = useRef<HTMLInputElement>(null);                // Скрытый input для файлов
  const menuRef = useRef<HTMLDivElement>(null);                       // Контекстное меню
  const modalRef = useRef<HTMLDivElement>(null);                      // Модальное окно

  // sortedAgents — агенты, отсортированные в порядке: Admin → Coder → Novice.
  // Используется для единообразного отображения в панели моделей и аватарках.
  const sortedAgents = [...agents].sort((a, b) => {
    const order = { admin: 1, coder: 2, novice: 3 };
    return (order[a.name as keyof typeof order] || 99) - (order[b.name as keyof typeof order] || 99);
  });

  useEffect(() => {
    fetchAgents();
    fetchModels();
    fetchProviders();
    fetchWorkspaces();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useEffect(() => {
    const chat = chats.find(c => c.id === currentChatId);
    if (chat) {
      setMessages(chat.messages);
    }
  }, [currentChatId, chats]);

  useEffect(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' });
  }, [messages]);

  useEffect(() => {
    const handleClickOutside = (event: MouseEvent) => {
      if (menuRef.current && !menuRef.current.contains(event.target as Node)) {
        setMenuChatId(null);
      }
      if (modalRef.current && !modalRef.current.contains(event.target as Node)) {
        setShowPromptModal(false);
      }
    };
    document.addEventListener('mousedown', handleClickOutside);
    return () => document.removeEventListener('mousedown', handleClickOutside);
  }, []);

  const fetchAgents = async () => {
    try {
      const res = await axios.get(API_BASE);
      setAgents(res.data);
    } catch (err) {
      console.error('Failed to fetch agents', err);
      // Если бэкенд недоступен — показываем агентов по умолчанию
      // чтобы аватарки и переключение работали даже без сервера
      if (agents.length === 0) {
        setAgents(DEFAULT_AGENTS);
      }
    }
  };

  const fetchModels = async () => {
    try {
      const res = await axios.get(MODELS_API);
      setModels(res.data);
    } catch (err) {
      console.error('Failed to fetch models', err);
    }
  };

  // fetchProviders — загрузка списка облачных LLM-провайдеров и их моделей.
  // Формирует словарь cloudModels: {провайдер → [модели]} для выпадающих списков.
  const fetchProviders = async () => {
    try {
      const res = await axios.get(PROVIDERS_API);
      setProviders(res.data);
      const cm: {[k: string]: string[]} = {};
      const cmd: {[k: string]: ModelDetailInfo[]} = {};
      for (const p of res.data) {
        if (p.models && p.models.length > 0) {
          cm[p.name] = p.models;
        }
        if (p.models_detail && p.models_detail.length > 0) {
          cmd[p.name] = p.models_detail;
        }
      }
      setCloudModels(cm);
      setCloudModelsDetail(cmd);
    } catch (err) {
      console.error('Failed to fetch providers', err);
      if (providers.length === 0) {
        setProviders([
          { name: 'ollama', enabled: true, models: [], hasKey: true },
          { name: 'openai', enabled: false, models: [], hasKey: false },
          { name: 'anthropic', enabled: false, models: [], hasKey: false },
          { name: 'yandexgpt', enabled: false, models: [], hasKey: false },
          { name: 'gigachat', enabled: false, models: [], hasKey: false },
          { name: 'openrouter', enabled: false, models: [], hasKey: false },
          { name: 'routeway', enabled: false, models: [], hasKey: false },
          { name: 'lmstudio', enabled: false, models: [], hasKey: false },
        ]);
      }
    }
  };

  const refreshProviders = async () => {
    setRefreshingProviders(true);
    try {
      await fetchProviders();
      await fetchModels();
      await fetchAgents();
    } finally {
      setRefreshingProviders(false);
    }
  };

  // fetchWorkspaces — загрузка списка рабочих пространств из бэкенда.
  const fetchWorkspaces = async () => {
    try {
      const res = await axios.get(WORKSPACES_API);
      setWorkspaces(res.data || []);
    } catch (err) {
      console.error('Failed to fetch workspaces', err);
    }
  };

  // createWorkspace — создание нового рабочего пространства через диалог prompt().
  // Запрашивает имя и опционально путь к директории на ПК.
  const createWorkspace = async () => {
    const name = prompt('Имя пространства:');
    if (!name || !name.trim()) return;
    const path = prompt('Путь к рабочей директории (необязательно):') || '';
    try {
      await axios.post(WORKSPACES_API, { name: name.trim(), path: path.trim() });
      fetchWorkspaces();
    } catch (err) {
      console.error('Failed to create workspace', err);
    }
  };

  const deleteWorkspace = async (id: number) => {
    try {
      await axios.delete(`${WORKSPACES_API}?id=${id}`);
      if (currentWorkspaceId === id) setCurrentWorkspaceId(null);
      fetchWorkspaces();
    } catch (err) {
      console.error('Failed to delete workspace', err);
    }
  };

  // saveProviderConfig — сохранение настроек облачного провайдера (API-ключ, URL и др.).
  // После сохранения обновляет список провайдеров для отображения нового статуса.
  const saveProviderConfig = async (providerName: string) => {
    setProviderSaving(true);
    setProviderError(null);
    setProviderHint(null);
    setProviderSuccess(null);
    try {
      const res = await axios.post(PROVIDERS_API, {
        provider: providerName,
        api_key: providerForm.api_key,
        base_url: providerForm.base_url,
        folder_id: providerForm.folder_id,
        scope: providerForm.scope,
        service_account_json: providerForm.service_account_json,
        enabled: true
      }, { timeout: 20000 });
      const modelsCount = res.data.models ? res.data.models.length : 0;
      setProviderSuccess(`Подключено! Доступно моделей: ${modelsCount}`);
      setTimeout(async () => {
        setProviderForm({api_key: '', base_url: '', folder_id: '', scope: '', service_account_json: ''});
        setProviderSuccess(null);
        await fetchProviders();
        await fetchModels();
      }, 2000);
    } catch (err: unknown) {
      const axiosErr = err as { response?: { data?: { error?: string; hint?: string } }; code?: string };
      if (axiosErr.code === 'ECONNABORTED') {
        setProviderError('Таймаут: сервер не ответил за 20 сек. Проверьте API-ключ и URL.');
      } else {
        const data = axiosErr.response?.data;
        setProviderError(data?.error || 'Не удалось подключить провайдер');
        setProviderHint(data?.hint || null);
      }
    } finally {
      setProviderSaving(false);
    }
  };

  const fetchPrompts = async (agent: string) => {
    try {
      const res = await axios.get(`${PROMPTS_API}?agent=${agent}`);
      return Array.isArray(res.data) ? res.data : [];
    } catch (err) {
      console.error('Failed to fetch prompts', err);
      return [];
    }
  };

  const loadPrompt = async (agent: string, filename: string) => {
    try {
      await axios.post(LOAD_PROMPT_API, { agent, filename });
      fetchAgents();
    } catch (err) {
      console.error('Failed to load prompt', err);
    }
  };

  const handleEditPrompt = async (agentName: string) => {
    const files = await fetchPrompts(agentName);
    setAvailablePrompts(Array.isArray(files) ? files : []);
    setModalAgent(agentName);
    setSelectedPrompt('');
    setPromptTab('edit');
    const agent = agents.find(a => a.name === agentName);
    setPromptText(agent?.prompt || '');
    setShowPromptModal(true);
  };

  const handleSelectPrompt = () => {
    if (selectedPrompt) {
      loadPrompt(modalAgent, selectedPrompt);
      setShowPromptModal(false);
    }
  };

  const savePromptText = async () => {
    setPromptSaveStatus('saving');
    setPromptSaveError('');
    try {
      await axios.post(UPDATE_PROMPT_API, { agent: modalAgent, prompt: promptText });
      setPromptSaveStatus('success');
      fetchAgents();
      setTimeout(() => {
        setShowPromptModal(false);
        setPromptSaveStatus('idle');
      }, 800);
    } catch (err) {
      const error = err as { response?: { data?: { error?: string } | string }, message?: string };
      const errorMsg = error.response?.data;
      const errorStr = typeof errorMsg === 'string' ? errorMsg : errorMsg?.error || error.message || 'Не удалось сохранить промпт';
      setPromptSaveStatus('error');
      setPromptSaveError(errorStr);
      console.error('Failed to save prompt', err);
    }
  };

  void _ragFactText; void _setRagFactText;

  const addRagFileChunks = async (fileName: string, content: string) => {
    try {
      await axios.post(`${RAG_API}/add`, {
        title: fileName,
        content: content,
        source: 'user-upload'
      });
    } catch (err) {
      console.error('Failed to add RAG file', err);
    }
    fetchRagStats();
  };

  const fetchRagStats = async () => {
    try {
      const res = await axios.get(`${RAG_API}/stats`);
      setRagStats(res.data);
    } catch (err) {
      console.error('Failed to fetch RAG stats', err);
    }
  };

  const fetchRagFiles = async () => {
    try {
      const res = await axios.get(`${RAG_API}/files`);
      setRagFiles(Array.isArray(res.data) ? res.data : []);
    } catch (err) {
      console.error('Failed to fetch RAG files', err);
      setRagFiles([]);
    }
  };

  const deleteRagFile = async (fileName: string) => {
    try {
      await axios.delete(`${RAG_API}/delete?name=${encodeURIComponent(fileName)}`);
      fetchRagFiles();
      fetchRagStats();
    } catch (err) {
      console.error('Failed to delete RAG file', err);
    }
  };

  const fetchLogs = async () => {
    setLogsLoading(true);
    try {
      const params = new URLSearchParams();
      if (logLevelFilter !== 'all') params.set('level', logLevelFilter);
      if (logServiceFilter !== 'all') params.set('service', logServiceFilter);
      params.set('limit', '100');
      const res = await axios.get(`${LOGS_API}?${params.toString()}`);
      const raw = Array.isArray(res.data) ? res.data : [];
      setSystemLogs(raw.filter((l: SystemLog) => l && l.Level && l.Message));
    } catch (err) {
      console.error('Failed to fetch logs', err);
    } finally {
      setLogsLoading(false);
    }
  };

  useEffect(() => {
    if (showLogsPanel) {
      fetchLogs();
      // Автообновление логов каждые 5 секунд
      const interval = setInterval(fetchLogs, 5000);
      return () => clearInterval(interval);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [showLogsPanel, logLevelFilter, logServiceFilter]);

  const resolveLog = async (logId: number, resolved: boolean) => {
    try {
      await axios.patch(`${LOGS_API}?id=${logId}&resolved=${resolved}`);
      fetchLogs();
    } catch (err) {
      console.error('Failed to resolve log', err);
    }
  };

  const updateAgentModel = async (agentName: string, model: string, provider?: string) => {
    try {
      const payload: {agent: string; model: string; provider?: string} = { agent: agentName, model };
      if (provider) payload.provider = provider;
      const res = await axios.post(UPDATE_MODEL_API, payload);
      if (res.data?.status === 'ok') {
        // Обновляем локально без запроса к серверу для мгновенного отклика
        setAgents(prev => prev.map(a => 
          a.name === agentName 
            ? { ...a, model: model, provider: provider || a.provider }
            : a
        ));
      }
    } catch (err) {
      console.error('Failed to update model', err);
    }
  };

  const uploadAvatar = async (agentName: string, file: File) => {
    setUploadingAvatar(agentName);
    const formData = new FormData();
    formData.append('file', file);
    try {
      await axios.post(`${AVATAR_UPLOAD_API}?agent=${agentName}`, formData, {
        headers: { 'Content-Type': 'multipart/form-data' }
      });
      fetchAgents();
    } catch (err) {
      console.error('Failed to upload avatar', err);
    } finally {
      setUploadingAvatar(null);
    }
  };

  const handleAvatarClick = (agentName: string) => {
    const input = document.createElement('input');
    input.type = 'file';
    input.accept = 'image/*';
    input.onchange = (e) => {
      const file = (e.target as HTMLInputElement).files?.[0];
      if (file) {
        uploadAvatar(agentName, file);
      }
    };
    input.click();
  };

  // extractAgentNames — извлечение имён агентов из текста сообщения.
  // Ищет синонимы из nameMap в тексте для определения адресата(ов).
  // Возвращает уникальный массив имён агентов (admin, coder, novice).
  const extractAgentNames = (text: string): string[] => {
    const lower = text.toLowerCase();
    const found: string[] = [];
    for (const [ru, en] of Object.entries(nameMap)) {
      if (lower.includes(ru)) {
        found.push(en);
      }
    }
    return [...new Set(found)];
  };

  // handleFileAttach — открытие диалога выбора файлов для прикрепления.
  const handleFileAttach = () => {
    fileInputRef.current?.click();
  };

  // onFilesSelected — обработка выбранных файлов через FileReader API.
  // Читает содержимое каждого файла как текст и добавляет в attachedFiles.
  const onFilesSelected = (e: React.ChangeEvent<HTMLInputElement>) => {
    const files = e.target.files;
    if (!files) return;
    Array.from(files).forEach(file => {
      const reader = new FileReader();
      reader.onload = () => {
        const content = reader.result as string;
        setAttachedFiles(prev => [...prev, { name: file.name, content }]);
      };
      reader.readAsText(file);
    });
    e.target.value = '';
  };

  const removeAttachedFile = (index: number) => {
    setAttachedFiles(prev => prev.filter((_, i) => i !== index));
  };

  // toggleVoiceInput — включение/выключение голосового ввода через Web Speech API.
  // Использует SpeechRecognition с языком ru-RU, непрерывное распознавание.
  // При распознавании текст добавляется в поле ввода.
  const toggleVoiceInput = () => {
    const SpeechRecognitionAPI = window.SpeechRecognition || window.webkitSpeechRecognition;
    if (!SpeechRecognitionAPI) {
      alert('Ваш браузер не поддерживает голосовой ввод');
      return;
    }

    if (isListening && recognitionRef.current) {
      recognitionRef.current.stop();
      setIsListening(false);
      return;
    }

    const recognition = new SpeechRecognitionAPI();
    recognition.lang = 'ru-RU';
    recognition.continuous = true;
    recognition.interimResults = true;

    recognition.onresult = (event: SpeechRecognitionEvent) => {
      let transcript = '';
      for (let i = event.resultIndex; i < event.results.length; i++) {
        transcript += event.results[i][0].transcript;
      }
      if (event.results[event.results.length - 1].isFinal) {
        setInput(prev => prev + (prev ? ' ' : '') + transcript);
      }
    };

    recognition.onerror = () => {
      setIsListening(false);
    };

    recognition.onend = () => {
      setIsListening(false);
    };

    recognitionRef.current = recognition;
    recognition.start();
    setIsListening(true);
  };

  // detectDiscussion — определение, запрашивает ли пользователь межагентное обсуждение.
  // Ищет ключевые слова: 'обсудите', 'дискуссия', 'поспорьте' и др.
  const detectDiscussion = (text: string): boolean => {
    const lower = text.toLowerCase();
    const keywords = ['обсудите', 'обсуждение', 'дискуссия', 'поспорьте', 'обсудить', 'обсуди'];
    return keywords.some(k => lower.includes(k));
  };

  // startDiscussion — запуск межагентного обсуждения.
  // Агенты по очереди высказываются по теме в заданное количество раундов.
  // Каждый агент видит историю предыдущих высказываний.
  // Аватар говорящего агента пульсирует во время его ответа.
  const startDiscussion = async (topic: string, participants: string[], rounds: number, currentMessages: Message[]): Promise<Message[]> => {
    let allMessages = [...currentMessages];
    let discussionHistory: Message[] = [{ role: 'user', content: topic }];

    for (let round = 0; round < rounds; round++) {
      for (const agentName of participants) {
        setSpeakingAgent(agentName);
        try {
          const res = await axios.post(API_BASE + 'chat', {
            messages: [...allMessages.filter(m => m.role !== 'system'), ...discussionHistory],
            agent: agentName
          });
          const respContent = res.data.error ? ('Ошибка: ' + res.data.error) : res.data.response;
          const agentModel = agents.find(a => a.name === agentName)?.model || '';
          const msg: Message = { role: 'assistant', content: respContent, agent: agentName, model: agentModel };
          allMessages = [...allMessages, msg];
          discussionHistory = [...discussionHistory, msg];
          setMessages(allMessages);
        } catch (err) {
          const error = err as { response?: { data?: { error?: string, detail?: string } }, message?: string };
          const detail = error.response?.data?.error || error.response?.data?.detail || error.message;
          const errMsg: Message = { role: 'assistant', content: 'Ошибка: ' + detail, agent: agentName, model: agents.find(a => a.name === agentName)?.model };
          allMessages = [...allMessages, errMsg];
          discussionHistory = [...discussionHistory, errMsg];
          setMessages(allMessages);
        }
      }
    }
    setSpeakingAgent(null);
    return allMessages;
  };

  // fetchRagContext — поиск релевантного контекста в базе знаний (memory-service).
  // Возвращает до 3 наиболее релевантных фрагментов, разделённых '---'.
  // Используется когда включён RAG-режим.
  const fetchRagContext = async (query: string): Promise<string> => {
    try {
      const res = await axios.post(`${MEMORY_API}/search`, {
        query,
        top_k: 3,
        include_files: true
      });
      if (res.data.results && res.data.results.length > 0) {
        return res.data.results.map((r: { text: string }) => r.text).join('\n---\n');
      }
    } catch (err) {
      console.error('RAG search failed', err);
    }
    return '';
  };

  // buildMessagesWithRag — формирование сообщений с RAG-контекстом.
  // Добавляет системное сообщение с контекстом из базы знаний перед вопросом пользователя.
  const buildMessagesWithRag = (userMsg: Message, context: string): Message[] => {
    if (!context) return [userMsg];
    const systemMsg: Message = {
      role: 'system',
      content: `Контекст из базы знаний:\n${context}\n\nИспользуй этот контекст для ответа на вопрос пользователя.`
    };
    return [systemMsg, userMsg];
  };

  // sendMessage — главная функция отправки сообщения.
  // Алгоритм:
  //  1. Формирование сообщения из текста + прикреплённых файлов
  //  2. RAG-поиск контекста (если включён RAG-режим)
  //  3. Определение адресатов по именам в тексте
  //  4. Три режима:
  //     a) Обсуждение — если обнаружены ключевые слова (обсудите, дискуссия)
  //     b) Одиночный запрос — если имена не упомянуты (отправка текущему агенту)
  //     c) Множественный запрос — если упомянуты конкретные агенты (последовательные ответы)
  //  5. Сохранение сообщений в чат и обновление превью
  const sendMessage = async () => {
    if (!input.trim() && attachedFiles.length === 0) return;

    const displayContent = input.trim();
    const currentFiles = [...attachedFiles];

    // Для отображения в чате: только текст пользователя (файлы показываются как значки)
    const userMsg: Message = {
      role: 'user',
      content: displayContent || currentFiles.map(f => f.name).join(', '),
      files: currentFiles.length > 0 ? currentFiles : undefined,
      timestamp: new Date().toLocaleString('ru-RU')
    };

    // Для отправки в API: текст + содержимое файлов (с chunking для больших файлов)
    let apiContent = input;
    if (currentFiles.length > 0) {
      const fileDescriptions = currentFiles.map(f => {
        if (f.content.length > CHUNK_SIZE) {
          const totalChunks = Math.ceil(f.content.length / CHUNK_SIZE);
          return `[Файл: ${f.name} (часть 1/${totalChunks}, ${f.content.length} символов)]\n${f.content.substring(0, CHUNK_SIZE)}\n[...обрезано. Скажите «продолжить чтение» для следующей части]`;
        }
        return `[Файл: ${f.name}]\n${f.content}`;
      }).join('\n\n');
      apiContent = apiContent ? `${apiContent}\n\n${fileDescriptions}` : fileDescriptions;
    }
    const updatedMessages = [...messages, userMsg];
    setMessages(updatedMessages);
    setInput('');
    setAttachedFiles([]);
    setLoading(true);

    // Сразу сохраняем сообщение пользователя в чат, чтобы useEffect не сбросил его
    updateChatMessages(currentChatId, updatedMessages);

    // Закрываем панели при отправке сообщения, чтобы пользователь видел чат
    setAgentsPanelOpen(false);
    setShowRagPanel(false);

    let context = '';
    if (ragEnabled) {
      context = await fetchRagContext(apiContent);
    }

    const mentioned = extractAgentNames(apiContent);
    let finalMessages: Message[] = updatedMessages;

    // История для API: предыдущие сообщения + текущее с содержимым файлов
    const historyForApi = updatedMessages.slice(0, -1).filter(m => m.role !== 'system').map(m => ({
      role: m.role,
      content: m.content
    }));
    historyForApi.push({ role: 'user', content: apiContent });

    if (detectDiscussion(apiContent)) {
      const participants = mentioned.length >= 2
        ? mentioned
        : mentioned.length === 1
          ? [...new Set([currentAgent, ...mentioned])]
          : agents.map(a => a.name);
      const systemMsg: Message = { role: 'system', content: `Начинается обсуждение между агентами: ${participants.join(', ')}` };
      finalMessages = [...finalMessages, systemMsg];
      setMessages(finalMessages);
      finalMessages = await startDiscussion(apiContent, participants, 3, finalMessages);
    } else if (mentioned.length === 0 || currentAgent === 'admin') {
      // Если выбран admin — всегда отправляем ему, даже если в тексте упомянуты другие агенты.
      // Admin сам делегирует задачи через составные скилы (delegate_tasks).
      setSpeakingAgent(currentAgent);
      try {
        const chatMessages = ragEnabled ? buildMessagesWithRag({ role: 'user', content: apiContent }, context) : historyForApi;
        const res = await axios.post(API_BASE + 'chat', {
          messages: chatMessages,
          agent: currentAgent
        });
        const curModel = agents.find(a => a.name === currentAgent)?.model || '';
        const content = res.data.error ? 'Ошибка: ' + res.data.error : (res.data.response || '(пустой ответ)');
        const assistantMsg: Message = { role: 'assistant', content, agent: currentAgent, model: curModel, sources: res.data.sources, timestamp: new Date().toLocaleString('ru-RU') };
        finalMessages = [...finalMessages, assistantMsg];
        setMessages(finalMessages);
      } catch (err) {
        const error = err as { response?: { data?: { error?: string } }, message?: string };
        const errorMsg: Message = { role: 'assistant', content: 'Ошибка: ' + (error.response?.data?.error || error.message), agent: currentAgent, model: agents.find(a => a.name === currentAgent)?.model, timestamp: new Date().toLocaleString('ru-RU') };
        finalMessages = [...finalMessages, errorMsg];
        setMessages(finalMessages);
      } finally {
        setSpeakingAgent(null);
      }
    } else {
      const lastAgent = mentioned[mentioned.length - 1];
      for (const agentName of mentioned) {
        setSpeakingAgent(agentName);
        try {
          const res = await axios.post(API_BASE + 'chat', {
            messages: historyForApi,
            agent: agentName
          });
          const mModel = agents.find(a => a.name === agentName)?.model || '';
          const content = res.data.error ? 'Ошибка: ' + res.data.error : (res.data.response || '(пустой ответ)');
          const assistantMsg: Message = { role: 'assistant', content, agent: agentName, model: mModel };
          finalMessages = [...finalMessages, assistantMsg];
          setMessages(finalMessages);
          historyForApi.push({ role: 'assistant', content });
        } catch (err) {
          const error = err as { response?: { data?: { error?: string } }, message?: string };
          const errorMsg: Message = { role: 'assistant', content: 'Ошибка: ' + (error.response?.data?.error || error.message), agent: agentName, model: agents.find(a => a.name === agentName)?.model };
          finalMessages = [...finalMessages, errorMsg];
          setMessages(finalMessages);
          historyForApi.push({ role: 'assistant', content: errorMsg.content });
        }
      }
      setSpeakingAgent(null);
      setCurrentAgent(lastAgent);
    }

    updateChatMessages(currentChatId, finalMessages);
    setLoading(false);
  };

  const updateChatMessages = (chatId: string, msgs: Message[]) => {
    setChats(prev => prev.map(chat =>
      chat.id === chatId ? { ...chat, messages: msgs, lastMessage: msgs[msgs.length-1]?.content } : chat
    ));
  };

  const handleKeyPress = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      sendMessage();
    }
  };

  const createNewChat = () => {
    const newId = Date.now().toString();
    setChats(prev => [...prev, { id: newId, name: 'Новый чат', messages: [], pinned: false }]);
    setCurrentChatId(newId);
    setMessages([]);
  };

  const selectChat = (id: string) => {
    setCurrentChatId(id);
    setMenuChatId(null);
  };

  const deleteChat = (id: string) => {
    if (chats.length <= 1) {
      alert('Нельзя удалить последний чат');
      return;
    }
    const newChats = chats.filter(c => c.id !== id);
    setChats(newChats);
    if (currentChatId === id) {
      setCurrentChatId(newChats[0].id);
      setMessages(newChats[0].messages);
    }
    setMenuChatId(null);
  };

  const renameChat = (id: string) => {
    const newName = prompt('Введите новое название чата:');
    if (newName && newName.trim()) {
      setChats(prev => prev.map(chat =>
        chat.id === id ? { ...chat, name: newName.trim() } : chat
      ));
    }
    setMenuChatId(null);
  };

  const pinChat = (id: string) => {
    setChats(prev => prev.map(chat =>
      chat.id === id ? { ...chat, pinned: !chat.pinned } : chat
    ));
    setMenuChatId(null);
  };

  const toggleMenu = (id: string, e: React.MouseEvent) => {
    e.stopPropagation();
    setMenuChatId(prev => (prev === id ? null : id));
  };

  const copyToClipboard = (text: string) => {
    navigator.clipboard.writeText(text);
  };

  // CodeBlock — компонент для отображения блоков кода с подсветкой синтаксиса.
  // Использует react-syntax-highlighter (тема vscDarkPlus) и кнопку копирования.
  const CodeBlock = ({ language, value }: { language: string; value: string }) => {
    return (
      <div className="code-block">
        <div className="code-block-header">
          <span className="code-language">{language}</span>
          <button className="copy-code-btn" onClick={() => copyToClipboard(value)}>⧉</button>
        </div>
        <div className="code-content">
          <SyntaxHighlighter 
            style={vscDarkPlus} 
            language={language} 
            PreTag="div"
            customStyle={{
              margin: 0,
              padding: 0,
              background: 'transparent',
            }}
          >
            {value}
          </SyntaxHighlighter>
        </div>
      </div>
    );
  };

  // sortedChats — чаты, отсортированные по закреплению (pinned сверху).
  const sortedChats = [...chats].sort((a, b) => {
    if (a.pinned && !b.pinned) return -1;
    if (!a.pinned && b.pinned) return 1;
    return 0;
  });

  // getAvatarForMessage — определение аватара для сообщения в чате.
  // Приоритет: 1) пользовательский аватар с сервера, 2) встроенная картинка из public/avatars/.
  const getAvatarForMessage = (msg: Message) => {
    if (msg.role === 'assistant' && msg.agent) {
      const agent = agents.find(a => a.name === msg.agent);
      if (agent?.avatar) {
        return <img src={`${AVATAR_BASE}${agent.avatar}`} alt={agent.name} />;
      }
      const builtIn = BUILT_IN_AVATARS[msg.agent];
      if (builtIn) return <img src={builtIn} alt={msg.agent} />;
    }
    const name = currentAgent || 'admin';
    const builtIn = BUILT_IN_AVATARS[name];
    if (builtIn) return <img src={builtIn} alt={name} />;
    return null;
  };

  return (
    <div className="container">
      <aside className="chats-sidebar">
        <div className="workspaces-section">
          <div className="workspaces-header">
            <h3>Пространства</h3>
            <button className="workspace-add-btn" onClick={createWorkspace}>+</button>
          </div>
          <div className="workspace-list">
            <div
              className={`workspace-item ${currentWorkspaceId === null ? 'active' : ''}`}
              onClick={() => setCurrentWorkspaceId(null)}
            >
              <span className="ws-icon">⌂</span>
              <span className="ws-name">Общее</span>
            </div>
            {workspaces.map(ws => (
              <div
                key={ws.ID}
                className={`workspace-item ${currentWorkspaceId === ws.ID ? 'active' : ''}`}
                onClick={() => setCurrentWorkspaceId(ws.ID)}
              >
                <span className="ws-icon">❐</span>
                <span className="ws-name">{ws.Name}</span>
                <button className="ws-delete" onClick={(e) => { e.stopPropagation(); deleteWorkspace(ws.ID); }}>✕</button>
              </div>
            ))}
          </div>
        </div>
        <div className="chats-header">
          <h2>Мои чаты</h2>
          <button className="new-chat-btn" onClick={createNewChat}>+</button>
        </div>
        <div className="chats-list">
          {sortedChats.map(chat => (
            <div
              key={chat.id}
              className={`chat-item ${currentChatId === chat.id ? 'active' : ''}`}
              onClick={() => selectChat(chat.id)}
            >
              <div className="chat-avatar">💬</div>
              <div className="chat-info">
                <div className="chat-name">
                  {chat.pinned && <span className="pin-icon" title="Закреплён">📌</span>} {chat.name}
                </div>
                <div className="chat-preview">{chat.lastMessage?.substring(0, 30) || 'Новый чат'}</div>
              </div>
              <button className="chat-menu-btn" onClick={(e) => toggleMenu(chat.id, e)}>⋯</button>
              {menuChatId === chat.id && (
                <div className="chat-menu" ref={menuRef} onClick={(e) => e.stopPropagation()}>
                  <div className="chat-menu-item pin" onClick={() => pinChat(chat.id)}>
                    {chat.pinned ? 'Открепить' : 'Закрепить'}
                  </div>
                  <div className="chat-menu-item" onClick={() => renameChat(chat.id)}>Переименовать</div>
                  <div className="chat-menu-item delete" onClick={() => deleteChat(chat.id)}>Удалить</div>
                </div>
              )}
            </div>
          ))}
        </div>
      </aside>

      <main className="chat-main">
        <div className="chat-header">
          <div className="agent-avatars">
            {sortedAgents.map(agent => (
              <div
                key={agent.name}
                className={`agent-avatar-mini 
                  ${currentAgent === agent.name ? 'active' : ''} 
                  ${speakingAgent === agent.name ? 'speaking' : ''}`}
                onClick={() => setCurrentAgent(agent.name)}
                title={agent.name}
              >
                {agent.avatar ? (
                  <img src={`${AVATAR_BASE}${agent.avatar}`} alt={agent.name} onError={(e) => { const t = e.target as HTMLImageElement; const b = BUILT_IN_AVATARS[agent.name]; if (b && t.src !== window.location.origin + b) { t.src = b; } else { t.style.display = 'none'; t.parentElement!.textContent = agent.name.charAt(0).toUpperCase(); } }} />
                ) : BUILT_IN_AVATARS[agent.name] ? (
                  <img src={BUILT_IN_AVATARS[agent.name]} alt={agent.name} onError={(e) => { (e.target as HTMLImageElement).style.display = 'none'; (e.target as HTMLImageElement).parentElement!.textContent = agent.name.charAt(0).toUpperCase(); }} />
                ) : agent.name.charAt(0).toUpperCase()}
              </div>
            ))}
          </div>
          <div style={{ display: 'flex', gap: '8px', alignItems: 'center' }}>
            <button
              className={`agents-toggle ${agentsPanelOpen ? 'open' : ''}`}
              onClick={() => setAgentsPanelOpen(!agentsPanelOpen)}
            >
              Модели
            </button>
            <button
              className={`agents-toggle ${showRagPanel ? 'open' : ''}`}
              onClick={() => { const next = !showRagPanel; setShowRagPanel(next); if (next) { fetchRagStats(); fetchRagFiles(); } }}
            >
              RAG
            </button>
            <button
              className={`agents-toggle ${showLogsPanel ? 'open' : ''}`}
              onClick={() => { const next = !showLogsPanel; setShowLogsPanel(next); if (next) fetchLogs(); }}
            >
              Логи
            </button>
          </div>
        </div>

        {agentsPanelOpen && (
          <div className="agent-panel">
            <div className="model-mode-toggle">
              <button
                className={`model-mode-btn ${modelMode === 'local' ? 'active' : ''}`}
                onClick={() => { setModelMode('local'); setSelectedProvider('ollama'); }}
              >
                Локальные
              </button>
              <button
                className={`model-mode-btn ${modelMode === 'cloud' ? 'active' : ''}`}
                onClick={() => { setModelMode('cloud'); const firstCloud = providers.find(p => !['ollama', 'lmstudio'].includes(p.name) && p.hasKey); setSelectedProvider(firstCloud ? firstCloud.name : 'openai'); }}
              >
                Облачные
              </button>
            </div>
            <div className="provider-selector">
              {modelMode === 'local' ? (
                ['ollama', 'lmstudio'].map(pName => {
                  const pInfo = providers.find(p => p.name === pName);
                  const mCount = pName === 'ollama' ? models.length : (pInfo?.models?.length || 0);
                  return (
                    <button
                      key={pName}
                      className={`provider-chip ${selectedProvider === pName ? 'active' : ''}`}
                      onClick={() => setSelectedProvider(pName)}
                    >
                      {pName === 'ollama' ? 'Ollama' : 'LM Studio'}
                      {mCount > 0 && <span className="provider-chip-count">{mCount}</span>}
                    </button>
                  );
                })
              ) : (
                providers.filter(p => !['ollama', 'lmstudio'].includes(p.name)).map(p => (
                  <button
                    key={p.name}
                    className={`provider-chip ${selectedProvider === p.name ? 'active' : ''}`}
                    onClick={() => setSelectedProvider(p.name)}
                  >
                    {p.name.toUpperCase()}
                    {p.hasKey && (p.models?.length || 0) > 0 && <span className="provider-chip-count">{p.models?.length}</span>}
                    {!p.hasKey && <span className="provider-chip-nokey">!</span>}
                  </button>
                ))
              )}
              <button className="provider-chip refresh" onClick={refreshProviders} disabled={refreshingProviders} title="Обновить список моделей">
                {refreshingProviders ? '...' : '\u21BB'}
              </button>
            </div>
            {sortedAgents.map(agent => {
              const modelInfo = models.find(m => m.name === agent.model);
              const isLocalProvider = ['ollama', 'lmstudio'].includes(selectedProvider);
              const showWarning = isLocalProvider && selectedProvider === 'ollama' && agent.supportsTools && modelInfo && !modelInfo.supportsTools;
              const roleNote = isLocalProvider && selectedProvider === 'ollama' ? (modelInfo?.roleNotes?.[agent.name] || '') : '';
              const isSuitable = modelInfo?.suitableRoles?.includes(agent.name) ?? true;
              const selectedProviderInfo = providers.find(p => p.name === selectedProvider);
              const providerModels = selectedProvider === 'ollama'
                ? models.map(m => m.name)
                : (selectedProviderInfo?.models || cloudModels[selectedProvider] || []);

              return (
                <div
                  key={agent.name}
                  className={`agent-card ${currentAgent === agent.name ? 'active' : ''}`}
                  onClick={() => setCurrentAgent(agent.name)}
                >
                  <span className="agent-avatar" onClick={(e) => { e.stopPropagation(); handleAvatarClick(agent.name); }}>
                    {uploadingAvatar === agent.name ? (
                      <span className="avatar-loading">⏳</span>
                    ) : agent.avatar ? (
                      <img src={`${AVATAR_BASE}${agent.avatar}`} alt={agent.name} onError={(e) => { const t = e.target as HTMLImageElement; const b = BUILT_IN_AVATARS[agent.name]; if (b && t.src !== window.location.origin + b) { t.src = b; } else { t.style.display = 'none'; t.parentElement!.textContent = agent.name.charAt(0).toUpperCase(); } }} />
                    ) : BUILT_IN_AVATARS[agent.name] ? (
                      <img src={BUILT_IN_AVATARS[agent.name]} alt={agent.name} onError={(e) => { (e.target as HTMLImageElement).style.display = 'none'; (e.target as HTMLImageElement).parentElement!.textContent = agent.name.charAt(0).toUpperCase(); }} />
                    ) : agent.name.charAt(0).toUpperCase()}
                  </span>
                  <span className="agent-name">{agent.name}</span>
                  <select
                    className="agent-model-select"
                    value={agent.model || ''}
                    onChange={(e) => {
                      const val = e.target.value;
                      if (val) updateAgentModel(agent.name, val, selectedProvider);
                    }}
                    onClick={(e) => e.stopPropagation()}
                  >
                    <option value="">{providerModels.length === 0 ? (selectedProvider === 'lmstudio' ? 'Нет моделей — нажмите ↻' : 'Нет моделей') : 'Выберите модель'}</option>
                    {selectedProvider === 'ollama' ? (
                      models.map(m => {
                        const suitable = m.suitableRoles?.includes(agent.name);
                        const prefix = suitable ? '\u2713 ' : '\u2717 ';
                        return <option key={m.name} value={m.name}>{prefix}{m.name}{m.family ? ` (${m.family}${m.parameterSize ? ' ' + m.parameterSize : ''})` : ''}</option>;
                      })
                    ) : (
                      providerModels.map(m => {
                        const detail = cloudModelsDetail[selectedProvider]?.find(d => d.id === m);
                        const prefix = detail ? (detail.is_available ? '\u2605 ' : '\u25CB ') : '';
                        const price = detail ? ` [${detail.pricing_info}]` : '';
                        return <option key={m} value={m}>{prefix}{m}{price}</option>;
                      })
                    )}
                  </select>
                  <span 
                    className="tool-warning" 
                    style={{ visibility: showWarning ? 'visible' : 'hidden' }}
                    onClick={(e) => { e.stopPropagation(); if (showWarning) alert('Выбранная модель не поддерживает вызов инструментов. Функции, требующие выполнения команд, не будут работать.'); }}
                  >
                    ⚠️
                  </span>
                  {selectedProvider === 'ollama' && modelInfo && roleNote && (
                    <div
                      className={`role-recommendation ${isSuitable ? 'suitable' : 'unsuitable'}`}
                      title={roleNote}
                      onClick={(e) => e.stopPropagation()}
                    >
                      {isSuitable ? '\u2713' : '\u2717'} {roleNote}
                    </div>
                  )}
                  <div className="agent-prompt-container">
                    <span className="agent-prompt-label">Prompt</span>
                    <button
                      className="icon-btn small"
                      onClick={(e) => {
                        e.stopPropagation();
                        handleEditPrompt(agent.name);
                      }}
                      title="Выбрать файл промпта"
                    >
                      ✎
                    </button>
                  </div>
                </div>
              );
            })}
            {selectedProvider && selectedProvider !== 'ollama' && (() => {
              const p = providers.find(pr => pr.name === selectedProvider);
              if (!p) return null;
              return (
              <div className="provider-config-section">
                <div style={{display: 'flex', alignItems: 'center', justifyContent: 'space-between'}}>
                  <h4 style={{margin: 0}}>{p.name.toUpperCase()} — настройка</h4>
                  <span style={{fontSize: '0.75rem', color: p.hasKey ? '#4caf50' : '#ff9800'}}>
                    {p.hasKey ? `подключен${p.models && p.models.length > 0 ? ` (${p.models.length} моделей)` : ''}` : 'не настроен'}
                  </span>
                </div>
                <div key={p.name} style={{marginBottom: '8px'}}>
                  <div style={{marginTop: '6px'}}>
                    {!['ollama', 'lmstudio'].includes(p.name) && (
                      <div className="provider-config-row">
                        <label>API Key:</label>
                        <div style={{flex: 1, display: 'flex', alignItems: 'center', gap: '6px'}}>
                          <input
                            type="password"
                            value={providerForm.api_key}
                            onChange={(e) => setProviderForm({...providerForm, api_key: e.target.value})}
                            placeholder={p.hasKey ? '••••••• (оставьте пустым, чтобы не менять)' : 'Введите API-ключ'}
                            style={{flex: 1}}
                          />
                          {p.hasKey && <span style={{color: '#4caf50', fontSize: '0.75rem', whiteSpace: 'nowrap'}}>Ключ сохранён</span>}
                        </div>
                      </div>
                    )}
                    {(p.name === 'yandexgpt') && (
                      <>
                        <div className="provider-config-row">
                          <label>Folder ID:</label>
                          <input
                            value={providerForm.folder_id}
                            onChange={(e) => setProviderForm({...providerForm, folder_id: e.target.value})}
                            placeholder="b1g... (авто из JSON ключа)"
                          />
                        </div>
                        <div className="provider-config-row" style={{alignItems: 'flex-start'}}>
                          <label style={{paddingTop: '6px'}}>JSON ключ:</label>
                          <div style={{flex: 1, display: 'flex', flexDirection: 'column', gap: '4px'}}>
                            <textarea
                              value={providerForm.service_account_json}
                              onChange={(e) => setProviderForm({...providerForm, service_account_json: e.target.value})}
                              placeholder='Вставьте содержимое authorized_key.json (вместо API-ключа)'
                              style={{minHeight: '80px', fontFamily: 'monospace', fontSize: '0.75rem', resize: 'vertical'}}
                            />
                            <div style={{display: 'flex', gap: '6px'}}>
                              <button
                                className="provider-save-btn"
                                style={{background: '#2c3e50', fontSize: '0.75rem', padding: '3px 8px'}}
                                onClick={() => {
                                  const input = document.createElement('input');
                                  input.type = 'file';
                                  input.accept = '.json';
                                  input.onchange = (ev) => {
                                    const file = (ev.target as HTMLInputElement).files?.[0];
                                    if (file) {
                                      const reader = new FileReader();
                                      reader.onload = (e) => {
                                        const text = e.target?.result as string;
                                        setProviderForm(prev => ({...prev, service_account_json: text}));
                                      };
                                      reader.readAsText(file);
                                    }
                                  };
                                  input.click();
                                }}
                              >
                                Загрузить .json файл
                              </button>
                              <span style={{color: '#7f8c8d', fontSize: '0.7rem', alignSelf: 'center'}}>или вставьте текст выше</span>
                            </div>
                          </div>
                        </div>
                      </>
                    )}
                    {(p.name === 'gigachat') && (
                      <div className="provider-config-row">
                        <label>Scope:</label>
                        <input
                          value={providerForm.scope}
                          onChange={(e) => setProviderForm({...providerForm, scope: e.target.value})}
                          placeholder="GIGACHAT_API_PERS"
                        />
                      </div>
                    )}
                    {['lmstudio'].includes(p.name) && (
                      <div className="provider-config-row">
                        <label>URL сервера:</label>
                        <input
                          value={providerForm.base_url}
                          onChange={(e) => setProviderForm({...providerForm, base_url: e.target.value})}
                          placeholder="http://localhost:1234/v1"
                        />
                      </div>
                    )}
                    {!['ollama', 'lmstudio'].includes(p.name) && (
                      <div className="provider-config-row">
                        <label>Base URL:</label>
                        <input
                          value={providerForm.base_url}
                          onChange={(e) => setProviderForm({...providerForm, base_url: e.target.value})}
                          placeholder="Опционально"
                        />
                      </div>
                    )}
                    {providerError && (
                      <div style={{background: '#2c1b1b', border: '1px solid #c0392b', borderRadius: '6px', padding: '8px', marginTop: '6px', maxHeight: '150px', overflowY: 'auto'}}>
                        <div style={{color: '#e74c3c', fontSize: '0.85rem', fontWeight: 500}}>Ошибка: {providerError}</div>
                        {providerHint && (
                          <div style={{color: '#bdc3c7', fontSize: '0.8rem', marginTop: '4px'}}>{providerHint}</div>
                        )}
                      </div>
                    )}
                    {providerSuccess && (
                      <div style={{background: '#1b2c1b', border: '1px solid #27ae60', borderRadius: '6px', padding: '8px', marginTop: '6px', color: '#2ecc71', fontSize: '0.85rem'}}>
                        {providerSuccess}
                      </div>
                    )}
                    <div style={{display: 'flex', gap: '6px', marginTop: '6px'}}>
                      <button
                        className="provider-save-btn"
                        onClick={() => saveProviderConfig(p.name)}
                        disabled={providerSaving}
                      >
                        {providerSaving ? 'Проверка...' : 'Сохранить'}
                      </button>
                      <button
                        className="provider-save-btn"
                        style={{background: '#2c3e50'}}
                        onClick={() => setProviderGuideOpen(providerGuideOpen === p.name ? null : p.name)}
                      >
                        {providerGuideOpen === p.name ? 'Скрыть справку' : 'Справка'}
                      </button>
                    </div>
                    {providerGuideOpen === p.name && p.guide && (
                      <div className="provider-guide-panel">
                        <div className="guide-tabs">
                          <button className={`guide-tab ${guideTab === 'connect' ? 'active' : ''}`} onClick={() => setGuideTab('connect')}>Подключение</button>
                          <button className={`guide-tab ${guideTab === 'choose' ? 'active' : ''}`} onClick={() => setGuideTab('choose')}>Выбор модели</button>
                          <button className={`guide-tab ${guideTab === 'pay' ? 'active' : ''}`} onClick={() => setGuideTab('pay')}>Оплата</button>
                          <button className={`guide-tab ${guideTab === 'balance' ? 'active' : ''}`} onClick={() => setGuideTab('balance')}>Баланс</button>
                        </div>
                        <div className="guide-content">
                          {guideTab === 'connect' && <pre>{p.guide.how_to_connect}</pre>}
                          {guideTab === 'choose' && <pre>{p.guide.how_to_choose}</pre>}
                          {guideTab === 'pay' && <pre>{p.guide.how_to_pay}</pre>}
                          {guideTab === 'balance' && <pre>{p.guide.how_to_balance}</pre>}
                        </div>
                      </div>
                    )}
                  </div>
                </div>
              </div>
              );
            })()}
          </div>
        )}

        {showRagPanel && (
          <div className="rag-panel">
            <div className="rag-panel-header">
              <h4>RAG — база знаний</h4>
              <label className="rag-toggle-label">
                <input
                  type="checkbox"
                  checked={ragEnabled}
                  onChange={() => setRagEnabled(!ragEnabled)}
                />
                Включить RAG
              </label>
            </div>
            <p className="rag-description">
              Когда RAG включён, агент ищет релевантную информацию в базе знаний перед ответом.
            </p>
            {ragStats && (
              <div style={{fontSize: '0.8rem', color: 'var(--icon-color)', marginBottom: '8px'}}>
                Фактов: {ragStats.facts_count} | Файлов: {ragStats.files_count}
              </div>
            )}
            <div style={{display: 'flex', gap: '6px', alignItems: 'center', marginBottom: '8px'}}>
              <button
                className="provider-save-btn"
                disabled={ragUploadStatus === 'uploading'}
                onClick={() => {
                  const supportedExtensions = ['.txt', '.md', '.markdown', '.json', '.jsonl', '.csv', '.html', '.htm', '.xml', '.yaml', '.yml', '.go', '.py', '.js', '.ts', '.java', '.c', '.cpp', '.h', '.hpp', '.rs', '.rb', '.php', '.swift', '.kt', '.sh', '.bash', '.zsh', '.sql', '.graphql', '.gql', '.dockerfile', '.toml', '.ini', '.conf', '.log'];
                  
                  // Сначала пробуем webkitdirectory для выбора папки
                  const folderInput = document.createElement('input');
                  folderInput.type = 'file';
                  folderInput.multiple = true;
                  // @ts-ignore
                  folderInput.webkitdirectory = true;
                  folderInput.style.display = 'none';
                  
                  folderInput.onchange = async () => {
                    const files = (folderInput as HTMLInputElement).files;
                    if (!files || files.length === 0) {
                      folderInput.remove();
                      return;
                    }
                    setRagUploadStatus('uploading');
                    setRagUploadMessage(`Загрузка ${files.length}...`);
                    let successCount = 0;
                    let skippedCount = 0;
                    
                    for (const file of Array.from(files)) {
                      const ext = '.' + (file.name.split('.').pop()?.toLowerCase() || '');
                      if (!supportedExtensions.includes(ext)) { skippedCount++; continue; }
                      try {
                        const content = await file.text();
                        const fileName = (file as any).webkitRelativePath || file.name;
                        await addRagFileChunks(fileName, content);
                        successCount++;
                      } catch (err) { console.error('Error:', file.name, err); }
                    }
                    await fetchRagFiles();
                    await fetchRagStats();
                    setRagUploadStatus('success');
                    let msg = `Загружено: ${successCount}`;
                    if (skippedCount > 0) msg += ` (пропущено: ${skippedCount})`;
                    setRagUploadMessage(msg);
                    setTimeout(() => { setRagUploadStatus('idle'); setRagUploadMessage(''); }, 3000);
                    folderInput.remove();
                  };
                  
                  document.body.appendChild(folderInput);
                  folderInput.click();
                }}
              >
                {ragUploadStatus === 'uploading' ? 'Загрузка...' : 'Добавить файлы'}
              </button>
              {ragUploadMessage && (
                <span style={{fontSize: '0.78rem', color: ragUploadStatus === 'success' ? '#4caf50' : ragUploadStatus === 'error' ? '#ff6b6b' : 'var(--icon-color)'}}>
                  {ragUploadMessage}
                </span>
              )}
            </div>
            <div style={{marginTop: '4px'}}>
              <div style={{fontSize: '0.8rem', color: 'var(--icon-color)', marginBottom: '4px', fontWeight: 500}}>Файлы в базе знаний:</div>
              {ragFiles.length > 0 ? (
                <div className="rag-file-list">
                  {ragFiles.map((folder: any, idx: number) => (
                    <div key={idx} className="rag-folder-group">
                      <div className="rag-folder-header">
                        <span className="rag-folder-icon">📁</span>
                        <span className="rag-folder-name">{folder.folder}</span>
                        <span className="rag-folder-count">({folder.total_files} файлов)</span>
                        <button className="rag-folder-delete" onClick={() => {
                          // Удаляем все файлы из папки
                          if (folder.files && folder.files.length > 0) {
                            folder.files.forEach((f: any) => deleteRagFile(folder.folder + '/' + f.file_name));
                            fetchRagFiles();
                            fetchRagStats();
                          }
                        }} title="Удалить папку">✕</button>
                      </div>
                      <div className="rag-folder-files">
                        {(folder.files || []).slice(0, 10).map((rf: any, fileIdx: number) => (
                          <div key={fileIdx} className="rag-file-item">
                            <span className="rag-file-icon">📄</span>
                            <span className="rag-file-name">{rf.file_name}</span>
                            <span className="rag-file-chunks">{rf.chunks_count} фр.</span>
                            <button className="rag-file-delete" onClick={() => deleteRagFile(folder.folder + '/' + rf.file_name)} title="Удалить">✕</button>
                          </div>
                        ))}
                        {(folder.files || []).length > 10 && (
                          <div className="rag-more-files">... и ещё {(folder.files || []).length - 10} файлов</div>
                        )}
                      </div>
                    </div>
                  ))}
                </div>
              ) : (
                <div style={{fontSize: '0.8rem', color: 'var(--icon-color)', fontStyle: 'italic', padding: '8px 0'}}>Нет загруженных файлов. Нажмите «Загрузить файл» для добавления.</div>
              )}
            </div>
          </div>
        )}

        <div className={`logs-slide-panel ${showLogsPanel ? 'open' : ''}`}>
          <div className="logs-slide-header">
            <h4 style={{margin: 0}}>Логи системы</h4>
            <button className="logs-close-btn" onClick={() => setShowLogsPanel(false)} title="Закрыть">&times;</button>
          </div>
          <div className="logs-toolbar">
            <select value={logLevelFilter} onChange={(e) => { setLogLevelFilter(e.target.value); }}>
              <option value="all">Все уровни</option>
              <option value="error">Error</option>
              <option value="warn">Warn</option>
              <option value="info">Info</option>
            </select>
            <select value={logServiceFilter} onChange={(e) => { setLogServiceFilter(e.target.value); }}>
              <option value="all">Все сервисы</option>
              <option value="agent-service">Agent</option>
              <option value="tools-service">Tools</option>
              <option value="memory-service">Memory</option>
              <option value="api-gateway">Gateway</option>
            </select>
            <button className="logs-refresh-btn" onClick={fetchLogs} disabled={logsLoading}>
              {logsLoading ? '...' : '\u21BB'}
            </button>
            <span className="logs-count">{systemLogs.length} записей</span>
          </div>
          <div className="logs-list">
            {systemLogs.length === 0 ? (
              <div className="logs-empty">Нет логов{logLevelFilter !== 'all' || logServiceFilter !== 'all' ? ' по выбранным фильтрам' : ''}</div>
            ) : (
              systemLogs.map((log, logIdx) => (
                <div key={log.ID ?? logIdx} className={`log-entry log-${log.Level || 'info'} ${log.Resolved ? 'log-resolved' : ''}`}>
                  <div className="log-header">
                    <span className={`log-level-badge log-badge-${log.Level || 'info'}`}>{(log.Level || 'info').toUpperCase()}</span>
                    <span className="log-service">{log.Service || '—'}</span>
                    <span className="log-time">{log.CreatedAt ? new Date(log.CreatedAt).toLocaleString('ru-RU', {day: '2-digit', month: '2-digit', hour: '2-digit', minute: '2-digit'}) : '—'}</span>
                    <button
                      className={`log-resolve-btn ${log.Resolved ? 'resolved' : ''}`}
                      onClick={() => resolveLog(log.ID, !log.Resolved)}
                      title={log.Resolved ? 'Пометить как нерешённое' : 'Пометить как решённое'}
                    >{log.Resolved ? '\u2713' : '\u25CB'}</button>
                  </div>
                  <div className="log-message">{log.Message}</div>
                  {log.Details && <div className="log-details">{log.Details}</div>}
                </div>
              ))
            )}
          </div>
        </div>

        <div className="chat-messages">
          {messages.map((msg, idx) => (
            <div key={idx} className={`message ${msg.role}`}>
              {msg.role === 'assistant' && (
                <div className="message-avatar">
                  {getAvatarForMessage(msg)}
                </div>
              )}
              <div className="message-content">
                {msg.files && msg.files.length > 0 && (
                  <div className="message-files">
                    {msg.files.map((f, i) => (
                      <span
                        key={i}
                        className="message-file-badge"
                        style={{cursor: 'pointer'}}
                        onClick={() => setViewingFile(f)}
                        title="Нажмите, чтобы просмотреть содержимое"
                      >📎 {f.name}</span>
                    ))}
                  </div>
                )}
                <ReactMarkdown
                  components={{
                    code(props: { inline?: boolean; className?: string; children?: React.ReactNode }) {
                      const { inline, className, children } = props;
                      const match = /language-(\w+)/.exec(className || '');
                      return !inline && match ? (
                        <CodeBlock
                          language={match[1]}
                          value={String(children).replace(/\n$/, '')}
                        />
                      ) : (
                        <code className={className}>
                          {children}
                        </code>
                      );
                    }
                  }}
                >
                  {msg.content}
                </ReactMarkdown>
                {(msg.timestamp) && (
                  <div className="message-timestamp">{msg.timestamp}</div>
                )}
                {msg.role === 'assistant' && msg.model && (
                  <div className="message-model-label">{msg.agent}: {msg.model}</div>
                )}
                {msg.sources && msg.sources.length > 0 && (
                  <div className="message-sources">
                    <div className="sources-title">Источники:</div>
                    {msg.sources.map((src, i) => (
                      <div key={i} className="source-item">
                        <span className="source-rank">#{src.score}</span>
                        <span className="source-title">{src.title}</span>
                      </div>
                    ))}
                  </div>
                )}
              </div>
            </div>
          ))}
          {loading && !speakingAgent && (
            <div className="message assistant">
              <div className="message-avatar">
                {(() => {
                  const agent = agents.find(a => a.name === currentAgent);
                  if (agent?.avatar) return <img src={`${AVATAR_BASE}${agent.avatar}`} alt={agent.name} />;
                  const builtIn = BUILT_IN_AVATARS[currentAgent];
                  if (builtIn) return <img src={builtIn} alt={currentAgent} />;
                  return currentAgent.charAt(0).toUpperCase();
                })()}
              </div>
              <div className="message-content">...</div>
            </div>
          )}
          <div ref={messagesEndRef} />
        </div>

        <div className="chat-input-container">
          {attachedFiles.length > 0 && (
            <div className="attached-files">
              {attachedFiles.map((file, idx) => (
                <span key={idx} className="attached-file-tag">
                  {file.name}
                  <button className="remove-file-btn" onClick={() => removeAttachedFile(idx)}>x</button>
                </span>
              ))}
            </div>
          )}
          <div className="input-wrapper">
            <input
              type="file"
              ref={fileInputRef}
              onChange={onFilesSelected}
              multiple
              style={{ display: 'none' }}
            />
            <textarea
              id="user-input"
              value={input}
              onChange={(e) => setInput(e.target.value)}
              onKeyDown={handleKeyPress}
              placeholder="Введите сообщение..."
              rows={1}
            />
            <div className="input-buttons-vertical">
              <button className="icon-btn" onClick={handleFileAttach} title="Прикрепить файл">📎</button>
              <button className="icon-btn" onClick={sendMessage} disabled={loading}>➤</button>
              <button id="mic-btn" className={`icon-btn ${isListening ? 'listening' : ''}`} onClick={toggleVoiceInput} title="Голосовой ввод">🎤</button>
            </div>
          </div>
        </div>
      </main>

      {viewingFile && (
        <div className="modal-overlay" onClick={() => setViewingFile(null)}>
          <div className="modal" onClick={(e) => e.stopPropagation()} style={{minWidth: '500px', maxWidth: '800px', maxHeight: '80vh', display: 'flex', flexDirection: 'column'}}>
            <div style={{display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '12px'}}>
              <h3 style={{margin: 0}}>{viewingFile.name}</h3>
              <button
                onClick={() => setViewingFile(null)}
                style={{background: 'transparent', border: 'none', color: 'var(--icon-color)', fontSize: '1.4rem', cursor: 'pointer', padding: '4px 8px'}}
              >✕</button>
            </div>
            <pre style={{flex: 1, overflow: 'auto', padding: '12px', borderRadius: '8px', border: '1px solid var(--input-border)', background: 'var(--input-bg)', color: 'var(--text-color)', fontSize: '0.85rem', fontFamily: "'Courier New', monospace", whiteSpace: 'pre-wrap', wordWrap: 'break-word', margin: 0, maxHeight: '60vh'}}>
              {viewingFile.content}
            </pre>
          </div>
        </div>
      )}

      {showPromptModal && (
        <div className="modal-overlay">
          <div className="modal" ref={modalRef} style={{minWidth: '500px', maxWidth: '700px'}}>
            <h3>Промпт агента: {modalAgent}</h3>
            <div style={{display: 'flex', gap: '4px', marginBottom: '12px'}}>
              <button
                className={`model-mode-btn ${promptTab === 'edit' ? 'active' : ''}`}
                onClick={() => setPromptTab('edit')}
                style={{padding: '6px 16px', borderRadius: '6px', border: '1px solid var(--input-border)', background: promptTab === 'edit' ? 'var(--input-focus-border)' : 'var(--button-bg)', color: promptTab === 'edit' ? '#fff' : 'var(--text-color)', cursor: 'pointer'}}
              >
                Редактировать
              </button>
              <button
                className={`model-mode-btn ${promptTab === 'files' ? 'active' : ''}`}
                onClick={() => setPromptTab('files')}
                style={{padding: '6px 16px', borderRadius: '6px', border: '1px solid var(--input-border)', background: promptTab === 'files' ? 'var(--input-focus-border)' : 'var(--button-bg)', color: promptTab === 'files' ? '#fff' : 'var(--text-color)', cursor: 'pointer'}}
              >
                Из файла
              </button>
            </div>
            <div className="modal-content" style={{maxHeight: '400px'}}>
              {promptTab === 'edit' ? (
                <div>
                  <p style={{fontSize: '0.85rem', color: 'var(--icon-color)', margin: '0 0 8px 0'}}>Системный промпт определяет поведение агента. Напишите инструкции для {modalAgent}:</p>
                  <textarea
                    value={promptText}
                    onChange={(e) => setPromptText(e.target.value)}
                    style={{width: '100%', minHeight: '200px', padding: '10px', borderRadius: '8px', border: '1px solid var(--input-border)', background: 'var(--input-bg)', color: 'var(--text-color)', fontSize: '0.9rem', fontFamily: 'inherit', resize: 'vertical', outline: 'none', boxSizing: 'border-box'}}
                    placeholder="Введите системный промпт для агента..."
                  />
                </div>
              ) : (
                <div>
                  <p style={{fontSize: '0.85rem', color: 'var(--icon-color)', margin: '0 0 8px 0'}}>Выберите готовый файл промпта из папки prompts/{modalAgent}/:</p>
                  {availablePrompts.length === 0 ? (
                    <p style={{color: 'var(--icon-color)'}}>Нет файлов в папке prompts/{modalAgent}/</p>
                  ) : (
                    <ul>
                      {availablePrompts.map(file => (
                        <li key={file}>
                          <label>
                            <input
                              type="radio"
                              name="promptFile"
                              value={file}
                              checked={selectedPrompt === file}
                              onChange={(e) => setSelectedPrompt(e.target.value)}
                            />
                            {file}
                          </label>
                        </li>
                      ))}
                    </ul>
                  )}
                  <div style={{marginTop: '12px', paddingTop: '12px', borderTop: '1px solid var(--input-border)'}}>
                    <p style={{fontSize: '0.85rem', color: 'var(--icon-color)', margin: '0 0 8px 0'}}>Или загрузите файл промпта с компьютера (.txt, .md):</p>
                    <button
                      style={{padding: '8px 16px', borderRadius: '6px', border: '1px solid var(--input-border)', background: 'var(--button-bg)', color: 'var(--text-color)', cursor: 'pointer'}}
                      onClick={() => {
                        const input = document.createElement('input');
                        input.type = 'file';
                        input.accept = '.txt,.md,.text';
                        input.onchange = async (e) => {
                          const file = (e.target as HTMLInputElement).files?.[0];
                          if (!file) return;
                          const reader = new FileReader();
                          reader.onload = async () => {
                            const content = reader.result as string;
                            setPromptText(content);
                            setPromptSaveStatus('saving');
                            try {
                              await axios.post(UPDATE_PROMPT_API, { agent: modalAgent, prompt: content });
                              setPromptSaveStatus('success');
                              fetchAgents();
                              setTimeout(() => { setShowPromptModal(false); setPromptSaveStatus('idle'); }, 800);
                            } catch (err) {
                              const error = err as { response?: { data?: { error?: string } } };
                              setPromptSaveStatus('error');
                              setPromptSaveError(error.response?.data?.error || 'Не удалось сохранить промпт');
                            }
                          };
                          reader.readAsText(file);
                        };
                        input.click();
                      }}
                    >
                      Загрузить с компьютера
                    </button>
                  </div>
                </div>
              )}
            </div>
            {promptSaveStatus === 'error' && (
              <div style={{color: '#ff6b6b', fontSize: '0.85rem', marginBottom: '8px'}}>{promptSaveError}</div>
            )}
            {promptSaveStatus === 'success' && (
              <div style={{color: '#4caf50', fontSize: '0.85rem', marginBottom: '8px'}}>Промпт сохранён!</div>
            )}
            <div className="modal-actions">
              <button onClick={() => { setShowPromptModal(false); setPromptSaveStatus('idle'); }}>Отмена</button>
              {promptTab === 'edit' ? (
                <button onClick={savePromptText} disabled={!promptText.trim() || promptSaveStatus === 'saving'}>
                  {promptSaveStatus === 'saving' ? 'Сохранение...' : 'Сохранить'}
                </button>
              ) : (
                <button onClick={handleSelectPrompt} disabled={!selectedPrompt}>Загрузить из файла</button>
              )}
            </div>
          </div>
        </div>
      )}
    </div>
  );
}

export default App;
