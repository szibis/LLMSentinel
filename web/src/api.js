// API client for Claude Escalate backend
import axios from 'axios';

// Use current origin for API calls (works whether running locally or in production)
const API_BASE_URL = import.meta.env.VITE_API_URL || `${window.location.origin}/api`;

const api = axios.create({
  baseURL: API_BASE_URL,
  timeout: 10000,
  headers: {
    'Content-Type': 'application/json',
  },
});

// Configuration endpoints
export const configAPI = {
  getConfig: () =>
    api.get('/config'),

  updateConfig: (config) =>
    api.post('/config', config),
};

// Status endpoints
export const statusAPI = {
  getStatus: () =>
    api.get('/status'),

  getHealth: () =>
    axios.get(`${window.location.origin}/health`),
};

// Optimizations endpoints
export const optimizationsAPI = {
  getOptimizations: () =>
    api.get('/optimizations'),

  toggleOptimization: (name, enabled) =>
    api.post(`/optimizations/${name}/toggle`, { enabled }),
};

// Cache control endpoints
export const cacheAPI = {
  getCacheStats: () =>
    api.get('/cache/stats'),

  clearCache: () =>
    api.post('/cache/clear'),
};

// Metrics endpoints
export const metricsAPI = {
  getMetrics: () =>
    api.get('/metrics'),

  streamMetrics: () => {
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    return `${protocol}//${window.location.host}/api/metrics/stream`;
  },
};

// Analytics endpoints (mock/derived from metrics)
export const analyticsAPI = {
  getTimeseries: (bucket, days) =>
    Promise.resolve({
      data: {
        timestamps: Array.from({ length: days }, (_, i) =>
          new Date(Date.now() - (days - i) * 24 * 60 * 60 * 1000).toISOString()
        ),
        values: Array.from({ length: days }, () =>
          Math.random() * 1000 + 500
        ),
      },
    }),

  getPercentiles: (bucket, days) =>
    Promise.resolve({
      data: {
        p50: Math.random() * 100 + 40,
        p95: Math.random() * 200 + 100,
        p99: Math.random() * 500 + 300,
      },
    }),

  getForecast: (metric, days) =>
    Promise.resolve({
      data: {
        forecast: Array.from({ length: days }, () =>
          Math.random() * 1000 + 500
        ),
        confidence: 0.85,
      },
    }),

  getTaskAccuracy: (days = 30) =>
    Promise.resolve({
      data: {
        accuracy: 0.94,
        precision: 0.96,
        recall: 0.92,
      },
    }),

  getCorrelations: () =>
    Promise.resolve({
      data: {
        cache_hit_rate: 0.85,
        avg_latency: 45.2,
        tokens_saved: 275000,
      },
    }),
};

export default api;
