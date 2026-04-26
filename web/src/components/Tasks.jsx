import React, { useState, useEffect } from 'react';
import { analyticsAPI } from '../api';

export default function Tasks({ darkMode }) {
  const [taskAccuracy, setTaskAccuracy] = useState([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    const fetchTasks = async () => {
      try {
        const response = await analyticsAPI.getTaskAccuracy(30);
        setTaskAccuracy(response.data || []);
      } catch (err) {
        console.error('Failed to load task accuracy:', err);
      } finally {
        setLoading(false);
      }
    };
    fetchTasks();
  }, []);

  return (
    <div className="space-y-8">
      <div>
        <h1 className={`text-4xl font-bold ${darkMode ? 'text-white' : 'text-gray-900'}`}>
          Task Classification
        </h1>
        <p className={`mt-2 ${darkMode ? 'text-gray-400' : 'text-gray-600'}`}>
          ML-based task type analysis and accuracy metrics
        </p>
      </div>

      {loading ? (
        <div className={`text-center py-12 ${darkMode ? 'text-gray-300' : 'text-gray-600'}`}>
          Loading task data...
        </div>
      ) : (
        <div className={`p-6 rounded-xl ${
          darkMode
            ? 'bg-gray-800/50 border border-gray-700'
            : 'bg-white/50 border border-white/60'
        } shadow-lg overflow-x-auto`}>
          <h2 className={`text-xl font-bold mb-4 ${darkMode ? 'text-white' : 'text-gray-900'}`}>
            Task-Model Accuracy
          </h2>

          <table className="w-full text-sm">
            <thead>
              <tr className={`border-b ${darkMode ? 'border-gray-700' : 'border-gray-300'}`}>
                <th className={`text-left py-3 px-4 font-medium ${darkMode ? 'text-gray-300' : 'text-gray-700'}`}>
                  Task Type
                </th>
                <th className={`text-left py-3 px-4 font-medium ${darkMode ? 'text-gray-300' : 'text-gray-700'}`}>
                  Model
                </th>
                <th className={`text-right py-3 px-4 font-medium ${darkMode ? 'text-gray-300' : 'text-gray-700'}`}>
                  Success Rate
                </th>
                <th className={`text-right py-3 px-4 font-medium ${darkMode ? 'text-gray-300' : 'text-gray-700'}`}>
                  Samples
                </th>
              </tr>
            </thead>
            <tbody>
              {taskAccuracy.length === 0 ? (
                <tr>
                  <td colSpan="4" className={`py-8 text-center ${darkMode ? 'text-gray-400' : 'text-gray-600'}`}>
                    No task accuracy data available yet
                  </td>
                </tr>
              ) : (
                taskAccuracy.map((task, idx) => (
                  <tr key={idx} className={`border-b ${darkMode ? 'border-gray-700' : 'border-gray-200'}`}>
                    <td className={`py-3 px-4 ${darkMode ? 'text-gray-300' : 'text-gray-900'}`}>
                      {task.task_type}
                    </td>
                    <td className={`py-3 px-4 ${darkMode ? 'text-gray-300' : 'text-gray-900'}`}>
                      {task.model}
                    </td>
                    <td className={`py-3 px-4 text-right`}>
                      <div className="flex items-center justify-end gap-2">
                        <div className={`w-24 h-2 rounded-full ${darkMode ? 'bg-gray-700' : 'bg-gray-300'}`}>
                          <div
                            className={`h-2 rounded-full ${
                              task.success_rate > 0.8 ? 'bg-green-500' : task.success_rate > 0.6 ? 'bg-yellow-500' : 'bg-red-500'
                            }`}
                            style={{ width: `${task.success_rate * 100}%` }}
                          />
                        </div>
                        <span className={`w-12 text-right ${darkMode ? 'text-gray-300' : 'text-gray-900'}`}>
                          {(task.success_rate * 100).toFixed(0)}%
                        </span>
                      </div>
                    </td>
                    <td className={`py-3 px-4 text-right ${darkMode ? 'text-gray-300' : 'text-gray-900'}`}>
                      {task.total_count}
                    </td>
                  </tr>
                ))
              )}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
