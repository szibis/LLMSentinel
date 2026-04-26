import React, { useState, useEffect } from 'react';
import { BrowserRouter as Router, Routes, Route, Link } from 'react-router-dom';
import Overview from './components/Overview';
import Analytics from './components/Analytics';
import Config from './components/Config';
import Tasks from './components/Tasks';
import Health from './components/Health';
import './index.css';

export default function App() {
  const [darkMode, setDarkMode] = useState(() => {
    const saved = localStorage.getItem('darkMode');
    return saved ? JSON.parse(saved) : false;
  });

  useEffect(() => {
    localStorage.setItem('darkMode', JSON.stringify(darkMode));
    if (darkMode) {
      document.documentElement.classList.add('dark');
    } else {
      document.documentElement.classList.remove('dark');
    }
  }, [darkMode]);

  return (
    <Router>
      <div className={`min-h-screen ${darkMode ? 'dark bg-gray-900' : 'bg-gradient-to-br from-slate-50 to-slate-100'}`}>
        {/* Navigation */}
        <nav className={`${darkMode ? 'bg-gray-800 border-gray-700' : 'bg-white border-b'} border-b shadow-sm sticky top-0 z-50`}>
          <div className="max-w-7xl mx-auto px-6 py-4">
            <div className="flex justify-between items-center">
              <div className="flex items-center gap-8">
                <Link to="/" className="flex items-center gap-2">
                  <div className="w-8 h-8 bg-gradient-to-br from-blue-500 to-purple-600 rounded-lg flex items-center justify-center">
                    <span className="text-white text-lg font-bold">⚡</span>
                  </div>
                  <span className={`font-bold text-xl ${darkMode ? 'text-white' : 'text-gray-900'}`}>
                    Claude Escalate
                  </span>
                </Link>

                <div className="flex gap-1">
                  {[
                    { path: '/', label: 'Overview', icon: '📊' },
                    { path: '/analytics', label: 'Analytics', icon: '📈' },
                    { path: '/tasks', label: 'Tasks', icon: '🎯' },
                    { path: '/config', label: 'Config', icon: '⚙️' },
                    { path: '/health', label: 'Health', icon: '💚' },
                  ].map(({ path, label, icon }) => (
                    <Link
                      key={path}
                      to={path}
                      className={`px-3 py-2 rounded-md text-sm font-medium transition-colors ${
                        darkMode
                          ? 'text-gray-300 hover:bg-gray-700 hover:text-white'
                          : 'text-gray-700 hover:bg-blue-50 hover:text-blue-600'
                      }`}
                    >
                      {icon} {label}
                    </Link>
                  ))}
                </div>
              </div>

              <button
                onClick={() => setDarkMode(!darkMode)}
                className={`p-2 rounded-lg transition-colors ${
                  darkMode
                    ? 'bg-gray-700 text-yellow-400 hover:bg-gray-600'
                    : 'bg-gray-200 text-gray-700 hover:bg-gray-300'
                }`}
                aria-label="Toggle dark mode"
              >
                {darkMode ? '☀️' : '🌙'}
              </button>
            </div>
          </div>
        </nav>

        {/* Main Content */}
        <main className="max-w-7xl mx-auto px-6 py-8">
          <Routes>
            <Route path="/" element={<Overview darkMode={darkMode} />} />
            <Route path="/analytics" element={<Analytics darkMode={darkMode} />} />
            <Route path="/tasks" element={<Tasks darkMode={darkMode} />} />
            <Route path="/config" element={<Config darkMode={darkMode} />} />
            <Route path="/health" element={<Health darkMode={darkMode} />} />
          </Routes>
        </main>

        {/* Footer */}
        <footer className={`${darkMode ? 'bg-gray-800 border-gray-700 text-gray-400' : 'bg-white border-t text-gray-500'} border-t py-6 mt-12`}>
          <div className="max-w-7xl mx-auto px-6 text-center text-sm">
            <p>Claude Escalate v4.0.0 • Save 40-99% on Claude API costs</p>
            <p className="mt-2">
              <a href="https://github.com/szibis/claude-escalate" className="hover:underline">
                GitHub
              </a>
              {' '} • {' '}
              <a href="https://github.com/szibis/claude-escalate/blob/main/docs/index.md" className="hover:underline">
                Docs
              </a>
            </p>
          </div>
        </footer>
      </div>
    </Router>
  );
}
