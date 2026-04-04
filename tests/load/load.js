import http from 'k6/http';
import { check, sleep } from 'k6';

const BASE_URL = __ENV.BASE_URL || 'http://localhost:7002';
const TOKEN = __ENV.TOKEN || '';
const TEAM_NAME = __ENV.TEAM_NAME || 'api-team';
const TARGET_GROUP = __ENV.TARGET_GROUP || 'public-api-gateway';
const ENV_NAME = __ENV.ENV_NAME || 'prod';

export const options = {
    vus: 500,
    duration: '30m',
    thresholds: {
        http_req_failed: ['rate<0.001'],
        // latency
        http_req_duration: [
            'p(95)<30',  // p95 < 50ms
            'p(99)<50',  // p99 < 50ms
        ],
    },
};

const headers = {
    // ...(TOKEN ? { Authorization: `Bearer ${TOKEN}` } : {}),
};

export default function () {
    const requests = [
        ['GET', `${BASE_URL}/api/v1/target-groups?team_name=${encodeURIComponent(TEAM_NAME)}`],
        ['GET', `${BASE_URL}/api/v1/whitelist/metrics?target_group=${encodeURIComponent(TARGET_GROUP)}&env=${encodeURIComponent(ENV_NAME)}`],
        ['GET', `${BASE_URL}/api/v1/whitelist/targets?target_group=${encodeURIComponent(TARGET_GROUP)}&env=${encodeURIComponent(ENV_NAME)}`],
        ['GET', `${BASE_URL}/api/v1/settings/metric-check-limits`],
        ['GET', `${BASE_URL}/api/v1/settings/scrape-tasks-schedule`],
    ];

    const [method, url] = requests[Math.floor(Math.random() * requests.length)];
    const res = http.request(method, url, null, { headers });

    check(res, {
        'status is 200': (r) => r.status === 200,
    });

    sleep(1);
}