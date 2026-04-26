import React, { useState, useEffect } from 'react';
import { analyticsAPI } from '../api';

export default function Analytics({ darkMode }) {
  const [timeseries, setTimeseries] = useState(null);
  const [forecast, setForecast] = useState(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    const fetchData = async () => {
      try {
        const [tsRes, fcRes] = await Promise.all([
          analyticsAPI.getTimeseries('daily', 30),
          analyticsAPI.getForecast('total_cost_usd', 7),
        ]);
        setTimeseries(tsRes.data);
        setForecast(fcRes.data);
      } catch (err) {
        console.error('Failed to load analytics:', err);
      } finally {
        setLoading(false);
      }
    };
    fetchData();
  }, []);

  return (
    <div className="space-y-8">
      <div>
        <h1 className={`text-4xl font-bold ${darkMode ? 'text-white' : 'text-gray-900'}`}>
          Analytics
        </h1>
        <p className={`mt-2 ${darkMode ? 'text-gray-400' : 'text-gray-600'}`}>
          Trends, forecasts, and insights
        </p>
      </div>

      {loading ? (
        <div className={`text-center py-12 ${darkMode ? 'text-gray-300' : 'text-gray-600'}`}>
          Loading analytics...
        </div>
      ) : (
        <div className={`p-6 rounded-xl ${
          darkMode
            ? 'bg-gray-800/50 border border-gray-700'
            : 'bg-white/50 border border-white/60'
        } shadow-lg`}>
          <h2 className={`text-xl font-bold mb-4 ${darkMode ? 'text-white' : 'text-gray-900'}`}>
            Cost Trends & Forecast
          </h2>
          <p className={darkMode ? 'text-gray-400' : 'text-gray-600'}>
            Time-series charts and forecasting coming in v4.1
          </p>
          {timeseries && (
            <pre className={`mt-4 p-4 rounded text-xs overflow-auto ${
              darkMode ? 'bg-gray-900 text-gray-300' : 'bg-gray-100 text-gray-800'
            }`}>
              {JSON.stringify(timeseries.slice(0, 3), null, 2)}
            </pre>
          )}
        </div>
      )}
    </div>
  );
}
