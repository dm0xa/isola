import React from 'react';
import { Routes, Route, Navigate } from 'react-router-dom';
import LandingPage from './pages/LandingPage';
import ViewerPage from './pages/ViewerPage';
import NetworkPage from './pages/NetworkPage';
import RunPage from './pages/RunPage';
import SharePage from './pages/SharePage';
import { ThemeProvider } from './lib/ThemeContext';
import { getPluginRoutes, usePlugins } from './plugins/registry';

function LazyFallback() {
  return <div className="flex items-center justify-center min-h-screen bg-zinc-200 dark:bg-gray-950" />;
}

export default function App() {
  const pluginVersion = usePlugins();
  const pluginRoutes = React.useMemo(() => {
    void pluginVersion;
    return getPluginRoutes();
  }, [pluginVersion]);

  return (
    <ThemeProvider>
      <React.Suspense fallback={<LazyFallback />}>
        <Routes>
          <Route path="/" element={<LandingPage />} />
          {pluginRoutes.map((r) => (
            <Route key={r.path} path={r.path} element={r.element} />
          ))}
          <Route path="/session/:id/network" element={<NetworkPage />} />
          <Route path="/session/:id" element={<ViewerPage />} />
          <Route path="/run" element={<RunPage />} />
          <Route path="/share/:token" element={<SharePage />} />
          <Route path="*" element={<Navigate to="/" replace />} />
        </Routes>
      </React.Suspense>
    </ThemeProvider>
  );
}
