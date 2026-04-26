import React, { useState, useEffect } from 'react';
import { metricsAPI, analyticsAPI } from '../api';

export default function Overview({ darkMode }) {
  const [metrics, setMetrics] = useState(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);

  useEffect(() => {
    const fetchMetrics = async () => {
      try {
        setLoading(true);
        const response = await metricsAPI.getMetricsSnapshot();
        setMetrics(response.data);
        setError(null);
      } catch (err) {
        setError('Failed to load metrics: ' + err.message);
      } finally {
        setLoading(false);
      }
    };

    fetchMetrics();
    const interval = setInterval(fetchMetrics, 5000); // Refresh every 5 seconds

    return () => clearInterval(interval);
  }, []);

  if (loading && !metrics) {
    return (
      <div className={`text-center py-12 ${darkMode ? 'text-gray-300' : 'text-gray-600'}`}>
        <div className="inline-block animate-spin">⏳</div>
        <p className="mt-2">Loading metrics...</p>
      </div>
    );
  }

  if (error) {
    return (
      <div className={`p-4 rounded-lg ${darkMode ? 'bg-red-900 text-red-100' : 'bg-red-100 text-red-700'}`}>
        {error}
      </div>
    );
  }

  const stats = metrics || {};
  const cacheHitRate = stats.cache_hit_rate ? (stats.cache_hit_rate * 100).toFixed(1) : 0;
  const costPerRequest = stats.cost_per_request ? stats.cost_per_request.toFixed(6) : 0;

  const StatCard = ({ label, value, unit = '', icon = '📊', trend = null }) => (
    <div className={`p-6 rounded-xl backdrop-blur-sm ${
      darkMode
        ? 'bg-gray-800/50 border border-gray-700'
        : 'bg-white/50 border border-white/60'
    } shadow-lg hover:shadow-xl transition-shadow`}>
      <div className="flex items-start justify-between">
        <div>
          <p className={`text-sm font-medium ${darkMode ? 'text-gray-400' : 'text-gray-600'}`}>
            {label}
          </p>
          <p className={`text-3xl font-bold mt-2 ${darkMode ? 'text-white' : 'text-gray-900'}`}>
            {value}{unit}
          </p>
          {trend && (
            <p className={`text-xs mt-2 ${trend > 0 ? 'text-green-500' : 'text-red-500'}`}>
              {trend > 0 ? '↑' : '↓'} {Math.abs(trend).toFixed(1)}% vs last period
            </p>
          )}
        </div>
        <div className="text-3xl">{icon}</div>
      </div>
    </div>
  );

  return (
    <div className="space-y-8">
      {/* Header */}
      <div>
        <h1 className={`text-4xl font-bold ${darkMode ? 'text-white' : 'text-gray-900'}`}>
          Dashboard
        </h1>
        <p className={`mt-2 ${darkMode ? 'text-gray-400' : 'text-gray-600'}`}>
          Real-time cost optimization metrics
        </p>
      </div>

      {/* Key Metrics Grid */}
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4">
        <StatCard
          label="Total Requests"
          value={stats.total_requests || 0}
          icon="🔄"
        />
        <StatCard
          label="Cache Hit Rate"
          value={cacheHitRate}
          unit="%"
          icon="💾"
          trend={15}
        />
        <StatCard
          label="Monthly Cost"
          value={stats.cost_this_month?.toFixed(2) || '0.00'}
          unit="$"
          icon="💰"
          trend={-5}
        />
        <StatCard
          label="Cost/Request"
          value={costPerRequest}
          unit="$"
          icon="📈"
        />
      </div>

      {/* Model Distribution */}
      <div className={`p-6 rounded-xl ${
        darkMode
          ? 'bg-gray-800/50 border border-gray-700'
          : 'bg-white/50 border border-white/60'
      } shadow-lg`}>
        <h2 className={`text-xl font-bold mb-4 ${darkMode ? 'text-white' : 'text-gray-900'}`}>
          Model Distribution
        </h2>
        <div className="grid grid-cols-3 gap-4">
          {['haiku', 'sonnet', 'opus'].map((model) => {
            const count = stats.model_usage?.[model] || 0;
            const percentage = stats.total_requests ? ((count / stats.total_requests) * 100).toFixed(1) : 0;
            return (
              <div key={model} className={`p-4 rounded-lg ${darkMode ? 'bg-gray-700' : 'bg-gray-100'}`}>
                <div className="flex items-center justify-between mb-2">
                  <span className={`font-medium capitalize ${darkMode ? 'text-gray-200' : 'text-gray-800'}`}>
                    {model}
                  </span>
                  <span className={`text-sm ${darkMode ? 'text-gray-400' : 'text-gray-600'}`}>
                    {percentage}%
                  </span>
                </div>
                <div className={`h-2 rounded-full ${darkMode ? 'bg-gray-600' : 'bg-gray-300'}`}>
                  <div
                    className={`h-2 rounded-full transition-all duration-300 ${
                      model === 'haiku' ? 'bg-blue-500' : model === 'sonnet' ? 'bg-purple-500' : 'bg-orange-500'
                    }`}
                    style={{ width: `${percentage}%` }}
                  />
                </div>
              </div>
            );
          })}
        </div>
      </div>

      {/* Quick Stats */}
      <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
        <div className={`p-6 rounded-xl ${
          darkMode
            ? 'bg-green-900/30 border border-green-700/50'
            : 'bg-green-100/30 border border-green-200'
        }`}>
          <p className={`text-sm font-medium ${darkMode ? 'text-green-300' : 'text-green-700'}`}>
            Queue Size
          </p>
          <p className={`text-2xl font-bold mt-2 ${darkMode ? 'text-green-300' : 'text-green-700'}`}>
            {stats.queue_size || 0}
          </p>
        </div>

        <div className={`p-6 rounded-xl ${
          darkMode
            ? 'bg-blue-900/30 border border-blue-700/50'
            : 'bg-blue-100/30 border border-blue-200'
        }`}>
          <p className={`text-sm font-medium ${darkMode ? 'text-blue-300' : 'text-blue-700'}`}>
            Cache Size
          </p>
          <p className={`text-2xl font-bold mt-2 ${darkMode ? 'text-blue-300' : 'text-blue-700'}`}>
            {stats.cache_size || 0}
          </p>
        </div>

        <div className={`p-6 rounded-xl ${
          darkMode
            ? 'bg-purple-900/30 border border-purple-700/50'
            : 'bg-purple-100/30 border border-purple-200'
        }`}>
          <p className={`text-sm font-medium ${darkMode ? 'text-purple-300' : 'text-purple-700'}`}>
            Active Sessions
          </p>
          <p className={`text-2xl font-bold mt-2 ${darkMode ? 'text-purple-300' : 'text-purple-700'}`}>
            {stats.active_sessions || 0}
          </p>
        </div>
      </div>

      {/* Latency Percentiles */}
      <div className={`p-6 rounded-xl ${
        darkMode
          ? 'bg-gray-800/50 border border-gray-700'
          : 'bg-white/50 border border-white/60'
      } shadow-lg`}>
        <h2 className={`text-xl font-bold mb-4 ${darkMode ? 'text-white' : 'text-gray-900'}`}>
          Request Latency Percentiles
        </h2>
        <div className="grid grid-cols-3 gap-4">
          <div className={`p-4 rounded-lg ${darkMode ? 'bg-gray-700' : 'bg-gray-100'}`}>
            <p className={`text-sm ${darkMode ? 'text-gray-400' : 'text-gray-600'}`}>P50</p>
            <p className={`text-2xl font-bold mt-1 ${darkMode ? 'text-white' : 'text-gray-900'}`}>
              {stats.latency_p50?.toFixed(0) || 0}ms
            </p>
          </div>
          <div className={`p-4 rounded-lg ${darkMode ? 'bg-gray-700' : 'bg-gray-100'}`}>
            <p className={`text-sm ${darkMode ? 'text-gray-400' : 'text-gray-600'}`}>P95</p>
            <p className={`text-2xl font-bold mt-1 ${darkMode ? 'text-white' : 'text-gray-900'}`}>
              {stats.latency_p95?.toFixed(0) || 0}ms
            </p>
          </div>
          <div className={`p-4 rounded-lg ${darkMode ? 'bg-gray-700' : 'bg-gray-100'}`}>
            <p className={`text-sm ${darkMode ? 'text-gray-400' : 'text-gray-600'}`}>P99</p>
            <p className={`text-2xl font-bold mt-1 ${darkMode ? 'text-white' : 'text-gray-900'}`}>
              {stats.latency_p99?.toFixed(0) || 0}ms
            </p>
          </div>
        </div>
      </div>
    </div>
  );
}
