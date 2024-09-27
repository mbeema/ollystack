import { BrowserRouter, Routes, Route } from 'react-router-dom';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import Layout from './components/Layout';
import DashboardPage from './pages/DashboardPage';
import CorrelationsPage from './pages/CorrelationsPage';
import TracesPage from './pages/TracesPage';
import TraceDetailPage from './pages/TraceDetailPage';
import LogsPage from './pages/LogsPage';
import MetricsPage from './pages/MetricsPage';
import ServiceMapPage from './pages/ServiceMapPage';
import AlertsPage from './pages/AlertsPage';
import ControlPlanePage from './pages/ControlPlanePage';
import SettingsPage from './pages/SettingsPage';

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 5000,
      retry: 1,
    },
  },
});

function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <BrowserRouter>
        <Routes>
          <Route path="/" element={<Layout />}>
            <Route index element={<DashboardPage />} />
            <Route path="correlations" element={<CorrelationsPage />} />
            <Route path="traces" element={<TracesPage />} />
            <Route path="traces/:traceId" element={<TraceDetailPage />} />
            <Route path="logs" element={<LogsPage />} />
            <Route path="metrics" element={<MetricsPage />} />
            <Route path="service-map" element={<ServiceMapPage />} />
            <Route path="alerts" element={<AlertsPage />} />
            <Route path="control-plane" element={<ControlPlanePage />} />
            <Route path="settings" element={<SettingsPage />} />
          </Route>
        </Routes>
      </BrowserRouter>
    </QueryClientProvider>
  );
}

export default App;
