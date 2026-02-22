// k6 нагрузочный тест для tools-service
// Запуск: k6 run tests/load/tools_load.js
//
// Тестирует: /health, /execute (безопасные команды), /system-info
// Пороговые значения: p(95) < 1000ms, ошибок < 10%

import http from 'k6/http';
import { check, sleep } from 'k6';
import { Rate, Trend } from 'k6/metrics';

const BASE_URL = __ENV.TOOLS_URL || 'http://localhost:8082';

const errorRate = new Rate('ошибки');
const latency = new Trend('задержка_мс');

export const options = {
  scenarios: {
    smoke: {
      executor: 'constant-vus',
      vus: 1,
      duration: '20s',
    },
    load: {
      executor: 'ramping-vus',
      startVUs: 0,
      stages: [
        { duration: '30s', target: 20 },
        { duration: '2m', target: 20 },
        { duration: '30s', target: 0 },
      ],
      startTime: '20s',
    },
  },
  thresholds: {
    http_req_duration: ['p(95)<1000'],
    'ошибки': ['rate<0.1'],
  },
};

export default function () {
  const params = {
    headers: {
      'Content-Type': 'application/json',
      'X-Request-ID': `k6-tools-${__VU}-${__ITER}`,
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

  const execPayload = JSON.stringify({ command: 'echo hello' });
  const execRes = http.post(`${BASE_URL}/execute`, execPayload, params);
  check(execRes, {
    'execute: статус 200': (r) => r.status === 200,
    'execute: stdout содержит hello': (r) => {
      try { return JSON.parse(r.body).stdout.includes('hello'); } catch { return false; }
    },
  });
  errorRate.add(execRes.status >= 400);
  latency.add(execRes.timings.duration);

  sleep(0.3);

  const infoRes = http.get(`${BASE_URL}/system-info`, params);
  check(infoRes, {
    'system-info: статус 200': (r) => r.status === 200,
  });
  errorRate.add(infoRes.status >= 400);
  latency.add(infoRes.timings.duration);

  sleep(0.5);
}
