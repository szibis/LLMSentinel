import React, { useState, useEffect } from 'react';
import { metricsAPI } from '../api';

export default function Health({ darkMode }) {
  const [health, setHealth] = useState(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    const checkHealth = async () => {
      try {
        const response = await metricsAPI.getHealth();
        setHealth(response.data);
      } catch (err) {
        setHealth({ status: 'unhealthy', error: err.message });
      } finally {
        setLoading(false);
      }
    };

    checkHealth();
    const interval = setInterval(checkHealth, 10000);
    return () => clearInterval(interval);
  }, []);

  const isHealthy = health?.status === 'healthy' || health?.status === 'ok';

  return (
    <div className="space-y-8">
      <div>
        <h1 className={`text-4xl font-bold ${darkMode ? 'text-white' : 'text-gray-900'}`}>
          Service Health
        </h1>
        <p className={`mt-2 ${darkMode ? 'text-gray-400' : 'text-gray-600'}`}>
          System status and diagnostics
        </p>
      </div>

      {loading ? (
        <div className={`text-center py-12 ${darkMode ? 'text-gray-300' : 'text-gray-600'}`}>
          Checking service health...
        </div>
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
          {/* Health Status */}
          <div className={`p-6 rounded-xl ${
            isHealthy
              ? darkMode
                ? 'bg-green-900/30 border border-green-700/50'
                : 'bg-green-100/30 border border-green-200'
              : darkMode
              ? 'bg-red-900/30 border border-red-700/50'
              : 'bg-red-100/30 border border-red-200'
          }`}>
            <div className="flex items-center gap-3">
              <div className={`w-4 h-4 rounded-full ${isHealthy ? 'bg-green-500' : 'bg-red-500'} animate-pulse`} />
              <div>
                <p className={`text-sm font-medium ${isHealthy ? (darkMode ? 'text-green-300' : 'text-green-700') : (darkMode ? 'text-red-300' : 'text-red-700')}`}>
                  Service Status
                </p>
                <p className={`text-2xl font-bold ${isHealthy ? (darkMode ? 'text-green-300' : 'text-green-700') : (darkMode ? 'text-red-300' : 'text-red-700')}`}>
                  {isHealthy ? 'Healthy' : 'Unhealthy'}
                </p>
              </div>
            </div>
          </div>

          {/* Uptime */}
          <div className={`p-6 rounded-xl ${
            darkMode
              ? 'bg-blue-900/30 border border-blue-700/50'
              : 'bg-blue-100/30 border border-blue-200'
          }`}>
            <p className={`text-sm font-medium ${darkMode ? 'text-blue-300' : 'text-blue-700'}`}>
              Uptime
            </p>
            <p className={`text-2xl font-bold ${darkMode ? 'text-blue-300' : 'text-blue-700'}`}>
              {health?.uptime_seconds ? `${Math.floor(health.uptime_seconds / 3600)}h` : 'Unknown'}
            </p>
          </div>

          {/* Details */}
          <div className={`col-span-1 md:col-span-2 p-6 rounded-xl ${
            darkMode
              ? 'bg-gray-800/50 border border-gray-700'
              : 'bg-white/50 border border-white/60'
          }`}>
            <h2 className={`text-lg font-bold mb-4 ${darkMode ? 'text-white' : 'text-gray-900'}`}>
              System Details
            </h2>
            <pre className={`p-4 rounded text-xs overflow-auto ${
              darkMode
                ? 'bg-gray-900 text-gray-300'
                : 'bg-gray-100 text-gray-800'
            }`}>
              {health ? JSON.stringify(health, null, 2) : 'No health data'}
            </pre>
          </div>
        </div>
      )}

      {/* Service Status Matrix */}
      <div className={`p-6 rounded-xl ${
        darkMode
          ? 'bg-gray-800/50 border border-gray-700'
          : 'bg-white/50 border border-white/60'
      }`}>
        <h2 className={`text-lg font-bold mb-4 ${darkMode ? 'text-white' : 'text-gray-900'}`}>
          Component Status
        </h2>
        <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
          {[
            { name: 'Main Service', status: isHealthy },
            { name: 'Database', status: isHealthy },
            { name: 'Metrics Export', status: isHealthy },
          ].map((comp) => (
            <div key={comp.name} className={`p-4 rounded-lg flex items-center gap-3 ${
              darkMode ? 'bg-gray-700' : 'bg-gray-100'
            }`}>
              <div className={`w-3 h-3 rounded-full ${comp.status ? 'bg-green-500' : 'bg-red-500'}`} />
              <span className={darkMode ? 'text-gray-300' : 'text-gray-700'}>
                {comp.name}
              </span>
              <span className={`ml-auto text-sm font-medium ${
                comp.status ? (darkMode ? 'text-green-400' : 'text-green-600') : (darkMode ? 'text-red-400' : 'text-red-600')
              }`}>
                {comp.status ? 'OK' : 'Down'}
              </span>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}
