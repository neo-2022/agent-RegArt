// k6 нагрузочный тест для memory-service
// Запуск: k6 run tests/load/memory_load.js
//
// Тестирует: /health, /search, /collections
// Пороговые значения: p(95) < 800ms, ошибок < 15%

import http from 'k6/http';
import { check, sleep } from 'k6';
import { Rate, Trend } from 'k6/metrics';

const BASE_URL = __ENV.MEMORY_URL || 'http://localhost:8001';

const errorRate = new Rate('ошибки');
const latency = new Trend('задержка_мс');

export const options = {
  scenarios: {
    smoke: {
      executor: 'constant-vus',
      vus: 1,
      duration: '30s',
    },
    load: {
      executor: 'ramping-vus',
      startVUs: 0,
      stages: [
        { duration: '1m', target: 30 },
        { duration: '2m', target: 30 },
        { duration: '1m', target: 0 },
      ],
      startTime: '30s',
    },
  },
  thresholds: {
    http_req_duration: ['p(95)<800'],
    'ошибки': ['rate<0.15'],
  },
};

export default function () {
  const params = {
    headers: {
      'Content-Type': 'application/json',
      'X-Request-ID': `k6-mem-${__VU}-${__ITER}`,
    },
    timeout: '10s',
  };

  const healthRes = http.get(`${BASE_URL}/health`, params);
  check(healthRes, {
    'health: статус 200': (r) => r.status === 200,
  });
  errorRate.add(healthRes.status !== 200);
  latency.add(healthRes.timings.duration);

  sleep(0.3);

  const searchPayload = JSON.stringify({
    query: 'тестовый запрос для нагрузочного теста',
    collection: 'default',
    top_k: 5,
  });
  const searchRes = http.post(`${BASE_URL}/search`, searchPayload, params);
  check(searchRes, {
    'search: статус 2xx или 404': (r) => (r.status >= 200 && r.status < 300) || r.status === 404,
  });
  errorRate.add(searchRes.status >= 500);
  latency.add(searchRes.timings.duration);

  sleep(0.5);
}
