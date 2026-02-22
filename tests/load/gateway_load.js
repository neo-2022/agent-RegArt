// k6 нагрузочный тест для API Gateway
// Запуск: k6 run tests/load/gateway_load.js
//
// Сценарии:
//   - smoke: быстрая проверка работоспособности (1 VU, 30 сек)
//   - load: нагрузочный тест (50 VU, рост → полка → спад)
//   - stress: стресс-тест (100 VU, проверка устойчивости)
//
// Пороговые значения:
//   - p(95) < 500ms — 95% запросов быстрее 500мс
//   - Ошибок < 10%

import http from 'k6/http';
import { check, sleep } from 'k6';
import { Rate, Trend } from 'k6/metrics';

const BASE_URL = __ENV.GATEWAY_URL || 'http://localhost:8080';

const errorRate = new Rate('ошибки');
const latency = new Trend('задержка_мс');

export const options = {
  scenarios: {
    smoke: {
      executor: 'constant-vus',
      vus: 1,
      duration: '30s',
      tags: { сценарий: 'smoke' },
    },
    load: {
      executor: 'ramping-vus',
      startVUs: 0,
      stages: [
        { duration: '1m', target: 50 },
        { duration: '3m', target: 50 },
        { duration: '1m', target: 0 },
      ],
      startTime: '30s',
      tags: { сценарий: 'load' },
    },
    stress: {
      executor: 'ramping-vus',
      startVUs: 0,
      stages: [
        { duration: '30s', target: 100 },
        { duration: '2m', target: 100 },
        { duration: '30s', target: 0 },
      ],
      startTime: '5m30s',
      tags: { сценарий: 'stress' },
    },
  },
  thresholds: {
    http_req_duration: ['p(95)<500'],
    'ошибки': ['rate<0.1'],
  },
};

export default function () {
  const params = {
    headers: {
      'Content-Type': 'application/json',
      'X-Request-ID': `k6-${__VU}-${__ITER}-${Date.now()}`,
    },
    timeout: '10s',
  };

  // GET /health — проверка работоспособности
  const healthRes = http.get(`${BASE_URL}/health`, params);
  check(healthRes, {
    'health: статус 200': (r) => r.status === 200,
    'health: ответ JSON': (r) => {
      try { JSON.parse(r.body); return true; } catch { return false; }
    },
  });
  errorRate.add(healthRes.status !== 200);
  latency.add(healthRes.timings.duration);

  sleep(0.5);

  // GET /models — список моделей
  const modelsRes = http.get(`${BASE_URL}/models`, params);
  check(modelsRes, {
    'models: статус 2xx': (r) => r.status >= 200 && r.status < 300,
  });
  errorRate.add(modelsRes.status >= 400);
  latency.add(modelsRes.timings.duration);

  sleep(0.5);

  // GET /agents/ — список агентов
  const agentsRes = http.get(`${BASE_URL}/agents/`, params);
  check(agentsRes, {
    'agents: статус 2xx': (r) => r.status >= 200 && r.status < 300,
  });
  errorRate.add(agentsRes.status >= 400);
  latency.add(agentsRes.timings.duration);

  sleep(0.5);
}
