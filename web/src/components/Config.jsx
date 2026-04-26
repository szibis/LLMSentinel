import React, { useState, useEffect } from 'react';
import { configAPI } from '../api';

export default function Config({ darkMode }) {
  const [config, setConfig] = useState(null);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [success, setSuccess] = useState(false);

  useEffect(() => {
    const fetchConfig = async () => {
      try {
        const response = await configAPI.getConfig();
        setConfig(response.data);
      } catch (err) {
        console.error('Failed to load config:', err);
      } finally {
        setLoading(false);
      }
    };
    fetchConfig();
  }, []);

  const handleSave = async () => {
    setSaving(true);
    try {
      await configAPI.updateConfig(config);
      setSuccess(true);
      setTimeout(() => setSuccess(false), 3000);
    } catch (err) {
      console.error('Failed to save config:', err);
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="space-y-8">
      <div>
        <h1 className={`text-4xl font-bold ${darkMode ? 'text-white' : 'text-gray-900'}`}>
          Configuration
        </h1>
        <p className={`mt-2 ${darkMode ? 'text-gray-400' : 'text-gray-600'}`}>
          Manage Claude Escalate settings
        </p>
      </div>

      {success && (
        <div className={`p-4 rounded-lg ${darkMode ? 'bg-green-900 text-green-100' : 'bg-green-100 text-green-700'}`}>
          ✓ Configuration saved successfully
        </div>
      )}

      <div className={`p-6 rounded-xl ${
        darkMode
          ? 'bg-gray-800/50 border border-gray-700'
          : 'bg-white/50 border border-white/60'
      } shadow-lg`}>
        <h2 className={`text-xl font-bold mb-6 ${darkMode ? 'text-white' : 'text-gray-900'}`}>
          Budgets
        </h2>

        {loading ? (
          <p className={darkMode ? 'text-gray-400' : 'text-gray-600'}>Loading...</p>
        ) : (
          <div className="space-y-4">
            <div>
              <label className={`block text-sm font-medium mb-2 ${darkMode ? 'text-gray-300' : 'text-gray-700'}`}>
                Daily Budget
              </label>
              <div className="flex gap-2">
                <span className={`flex items-center px-3 ${darkMode ? 'bg-gray-700 text-gray-300' : 'bg-gray-200 text-gray-700'}`}>
                  $
                </span>
                <input
                  type="number"
                  value={config?.budgets?.daily || 10}
                  onChange={(e) => setConfig({
                    ...config,
                    budgets: { ...config.budgets, daily: parseFloat(e.target.value) }
                  })}
                  className={`flex-1 px-3 py-2 rounded-lg ${
                    darkMode
                      ? 'bg-gray-700 text-white border-gray-600'
                      : 'bg-white text-gray-900 border-gray-300'
                  } border`}
                />
              </div>
            </div>

            <div>
              <label className={`block text-sm font-medium mb-2 ${darkMode ? 'text-gray-300' : 'text-gray-700'}`}>
                Monthly Budget
              </label>
              <div className="flex gap-2">
                <span className={`flex items-center px-3 ${darkMode ? 'bg-gray-700 text-gray-300' : 'bg-gray-200 text-gray-700'}`}>
                  $
                </span>
                <input
                  type="number"
                  value={config?.budgets?.monthly || 200}
                  onChange={(e) => setConfig({
                    ...config,
                    budgets: { ...config.budgets, monthly: parseFloat(e.target.value) }
                  })}
                  className={`flex-1 px-3 py-2 rounded-lg ${
                    darkMode
                      ? 'bg-gray-700 text-white border-gray-600'
                      : 'bg-white text-gray-900 border-gray-300'
                  } border`}
                />
              </div>
            </div>

            <button
              onClick={handleSave}
              disabled={saving}
              className={`w-full mt-6 py-2 px-4 rounded-lg font-medium transition-colors ${
                darkMode
                  ? 'bg-blue-600 hover:bg-blue-700 text-white disabled:bg-gray-700'
                  : 'bg-blue-500 hover:bg-blue-600 text-white disabled:bg-gray-400'
              }`}
            >
              {saving ? 'Saving...' : 'Save Configuration'}
            </button>
          </div>
        )}
      </div>

      <div className={`p-6 rounded-xl ${
        darkMode
          ? 'bg-blue-900/30 border border-blue-700/50'
          : 'bg-blue-100/30 border border-blue-200'
      }`}>
        <p className={`text-sm ${darkMode ? 'text-blue-300' : 'text-blue-700'}`}>
          💡 Tip: Configure budgets and OTEL endpoints to optimize your setup.
          Advanced settings like embedding models coming in v4.1.
        </p>
      </div>
    </div>
  );
}
