import { Outlet, NavLink } from 'react-router-dom';
import {
  LayoutDashboard,
  GitBranch,
  FileText,
  BarChart3,
  Network,
  Bell,
  Settings,
  Search,
  MessageSquare,
  Link2,
  ChevronLeft,
  ChevronRight,
  Cpu,
} from 'lucide-react';
import { useState, useEffect } from 'react';
import clsx from 'clsx';
import AICopilot from './AICopilot';

const navigation = [
  { name: 'Dashboard', href: '/', icon: LayoutDashboard },
  { name: 'Correlations', href: '/correlations', icon: Link2 },
  { name: 'Traces', href: '/traces', icon: GitBranch },
  { name: 'Logs', href: '/logs', icon: FileText },
  { name: 'Metrics', href: '/metrics', icon: BarChart3 },
  { name: 'Service Map', href: '/service-map', icon: Network },
  { name: 'Alerts', href: '/alerts', icon: Bell },
  { name: 'Control Plane', href: '/control-plane', icon: Cpu },
  { name: 'Settings', href: '/settings', icon: Settings },
];

export default function Layout() {
  const [copilotOpen, setCopilotOpen] = useState(false);
  const [sidebarCollapsed, setSidebarCollapsed] = useState(() => {
    const saved = localStorage.getItem('sidebar-collapsed');
    return saved === 'true';
  });

  useEffect(() => {
    localStorage.setItem('sidebar-collapsed', String(sidebarCollapsed));
  }, [sidebarCollapsed]);

  const sidebarWidth = sidebarCollapsed ? 'w-16' : 'w-64';
  const mainPadding = sidebarCollapsed ? 'pl-16' : 'pl-64';

  return (
    <div className="min-h-screen bg-gray-900 text-white">
      {/* Sidebar */}
      <div className={clsx(
        'fixed inset-y-0 left-0 bg-gray-800 border-r border-gray-700 transition-all duration-300 z-20',
        sidebarWidth
      )}>
        {/* Logo */}
        <div className={clsx(
          'flex items-center h-16 border-b border-gray-700 transition-all duration-300',
          sidebarCollapsed ? 'px-4 justify-center' : 'px-6'
        )}>
          {sidebarCollapsed ? (
            <span className="text-xl font-bold text-blue-400">O</span>
          ) : (
            <span className="text-xl font-bold text-blue-400">OllyStack</span>
          )}
        </div>

        {/* Collapse Toggle */}
        <button
          onClick={() => setSidebarCollapsed(!sidebarCollapsed)}
          className={clsx(
            'absolute top-20 -right-3 w-6 h-6 bg-gray-700 border border-gray-600 rounded-full flex items-center justify-center hover:bg-gray-600 transition-colors z-30',
            'text-gray-400 hover:text-white'
          )}
          title={sidebarCollapsed ? 'Expand sidebar' : 'Collapse sidebar'}
        >
          {sidebarCollapsed ? (
            <ChevronRight className="w-4 h-4" />
          ) : (
            <ChevronLeft className="w-4 h-4" />
          )}
        </button>

        {/* Navigation */}
        <nav className={clsx('mt-6', sidebarCollapsed ? 'px-2' : 'px-3')}>
          {navigation.map((item) => (
            <NavLink
              key={item.name}
              to={item.href}
              title={sidebarCollapsed ? item.name : undefined}
              className={({ isActive }) =>
                clsx(
                  'flex items-center mt-1 rounded-lg text-sm font-medium transition-all',
                  sidebarCollapsed ? 'px-3 py-3 justify-center' : 'px-3 py-2',
                  isActive
                    ? 'bg-blue-600 text-white'
                    : 'text-gray-300 hover:bg-gray-700 hover:text-white'
                )
              }
            >
              <item.icon className={clsx('w-5 h-5', !sidebarCollapsed && 'mr-3')} />
              {!sidebarCollapsed && item.name}
            </NavLink>
          ))}
        </nav>

        {/* AI Copilot Button */}
        <div className={clsx(
          'absolute bottom-4 transition-all duration-300',
          sidebarCollapsed ? 'left-2 right-2' : 'left-3 right-3'
        )}>
          <button
            onClick={() => setCopilotOpen(true)}
            title={sidebarCollapsed ? 'AI Copilot' : undefined}
            className={clsx(
              'flex items-center justify-center w-full bg-gradient-to-r from-purple-600 to-blue-600 rounded-lg text-sm font-medium hover:from-purple-700 hover:to-blue-700 transition-all',
              sidebarCollapsed ? 'px-2 py-3' : 'px-4 py-3'
            )}
          >
            <MessageSquare className={clsx('w-5 h-5', !sidebarCollapsed && 'mr-2')} />
            {!sidebarCollapsed && 'AI Copilot'}
          </button>
        </div>
      </div>

      {/* Main content */}
      <div className={clsx('transition-all duration-300', mainPadding)}>
        {/* Header */}
        <header className="h-16 bg-gray-800 border-b border-gray-700 flex items-center justify-between px-6">
          <div className="flex items-center flex-1">
            <div className="relative w-96">
              <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-5 h-5 text-gray-400" />
              <input
                type="text"
                placeholder="Search traces, logs, metrics..."
                className="w-full pl-10 pr-4 py-2 bg-gray-700 border border-gray-600 rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent"
              />
            </div>
          </div>

          <div className="flex items-center space-x-4">
            {/* Time range selector */}
            <select className="px-3 py-2 bg-gray-700 border border-gray-600 rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-blue-500">
              <option>Last 15 minutes</option>
              <option>Last 1 hour</option>
              <option>Last 6 hours</option>
              <option>Last 24 hours</option>
              <option>Last 7 days</option>
              <option>Custom range</option>
            </select>

            {/* Refresh button */}
            <button className="px-3 py-2 bg-gray-700 border border-gray-600 rounded-lg text-sm hover:bg-gray-600 transition-colors">
              Refresh
            </button>
          </div>
        </header>

        {/* Page content */}
        <main className="p-6">
          <Outlet />
        </main>
      </div>

      {/* AI Copilot Panel */}
      <AICopilot isOpen={copilotOpen} onClose={() => setCopilotOpen(false)} />
    </div>
  );
}
