export { default as DashboardPage } from './DashboardPage';
export { default as TracesPage } from './TracesPage';
export { default as ServiceMapPage } from './ServiceMapPage';

// Placeholder pages - implement as needed
export function TraceDetailPage() {
  return <div className="p-6"><h1 className="text-2xl font-bold">Trace Detail</h1></div>;
}

export function LogsPage() {
  return <div className="p-6"><h1 className="text-2xl font-bold">Logs</h1></div>;
}

export function MetricsPage() {
  return <div className="p-6"><h1 className="text-2xl font-bold">Metrics</h1></div>;
}

export function AlertsPage() {
  return <div className="p-6"><h1 className="text-2xl font-bold">Alerts</h1></div>;
}

export function SettingsPage() {
  return <div className="p-6"><h1 className="text-2xl font-bold">Settings</h1></div>;
}
