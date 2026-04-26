// API client for Claude Escalate backend
import axios from 'axios';

const API_BASE_URL = import.meta.env.VITE_API_URL || 'http://localhost:9000/api';

const api = axios.create({
  baseURL: API_BASE_URL,
  timeout: 10000,
  headers: {
    'Content-Type': 'application/json',
  },
});

// Analytics endpoints
export const analyticsAPI = {
  getTimeseries: (bucket, days) =>
    api.get('/analytics/timeseries', { params: { bucket, days } }),

  getPercentiles: (bucket, days) =>
    api.get('/analytics/percentiles', { params: { bucket, days } }),

  getForecast: (metric, days) =>
    api.get('/analytics/forecast', { params: { metric, days } }),

  getTaskAccuracy: (days = 30) =>
    api.get('/analytics/task-accuracy', { params: { days } }),

  getCorrelations: () =>
    api.get('/analytics/correlations'),
};

// Config endpoints
export const configAPI = {
  getConfig: () =>
    api.get('/config'),

  updateConfig: (config) =>
    api.post('/config', config),

  getBudgets: () =>
    api.get('/config/budgets'),

  setBudget: (budgetType, limit) =>
    api.post('/config/budgets', { type: budgetType, limit }),
};

// Metrics endpoints
export const metricsAPI = {
  getMetrics: () =>
    api.get('/metrics'),

  getMetricsSnapshot: () =>
    api.get('/metrics/snapshot'),

  getHealth: () =>
    api.get('/health'),
};

// Classify endpoint
export const classifyAPI = {
  predict: (prompt) =>
    api.post('/classify/predict', { prompt }),
};

export default api;
