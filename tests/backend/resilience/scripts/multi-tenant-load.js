import http from 'k6/http'
import { check } from 'k6'

export const options = {
  vus: Number(__ENV.RESILIENCE_VUS || 20),
  duration: __ENV.RESILIENCE_DURATION || '5m',
  thresholds: {
    http_req_failed: ['rate<0.005'],
    http_req_duration: ['p(95)<=1000', 'p(99)<=2500'],
  },
}

export default function () {
  const tenants = (__ENV.RESILIENCE_TENANTS || '').split(',').filter(Boolean)
  if (!tenants.length) throw new Error('RESILIENCE_TENANTS is required')
  const tenant = tenants[__ITER % tenants.length]
  const response = http.get(`${__ENV.RESILIENCE_TARGET_URL}/health/ready`, { headers: { 'X-Tenant-Code': tenant } })
  check(response, { ready: r => r.status === 200 })
}
