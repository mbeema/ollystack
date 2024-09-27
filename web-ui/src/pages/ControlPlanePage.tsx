import { useState, useEffect } from 'react';
import {
  Server,
  Settings2,
  Layers,
  Globe,
  Activity,
  CheckCircle,
  XCircle,
  AlertTriangle,
  Clock,
  RefreshCw,
  Plus,
  Trash2,
  Edit,
  Play,
  ChevronRight,
  Terminal,
  Cpu,
  HardDrive,
  Wifi,
  WifiOff,
  X,
  Copy,
  Download,
  Upload,
} from 'lucide-react';
import clsx from 'clsx';
import { opampApi, type Environment, type Group, type Config, type Agent, type FleetStatus, type ConfigTemplate } from '../lib/api';

// Modal Component
function Modal({ isOpen, onClose, title, children }: { isOpen: boolean; onClose: () => void; title: string; children: React.ReactNode }) {
  if (!isOpen) return null;

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center">
      <div className="absolute inset-0 bg-black/60 backdrop-blur-sm" onClick={onClose} />
      <div className="relative bg-gray-800 rounded-xl border border-gray-700 w-full max-w-2xl max-h-[90vh] overflow-hidden shadow-xl">
        <div className="flex items-center justify-between p-4 border-b border-gray-700">
          <h2 className="text-lg font-semibold">{title}</h2>
          <button onClick={onClose} className="p-1.5 hover:bg-gray-700 rounded-lg transition-colors">
            <X className="w-5 h-5" />
          </button>
        </div>
        <div className="p-6 overflow-y-auto max-h-[calc(90vh-120px)]">
          {children}
        </div>
      </div>
    </div>
  );
}

type TabType = 'overview' | 'agents' | 'configs' | 'groups' | 'environments';

const defaultOtelConfig = `receivers:
  otlp:
    protocols:
      grpc:
        endpoint: 0.0.0.0:4317
      http:
        endpoint: 0.0.0.0:4318

processors:
  batch:
    timeout: 1s
    send_batch_size: 1024
  memory_limiter:
    check_interval: 1s
    limit_mib: 512

exporters:
  otlp:
    endpoint: \${OTEL_EXPORTER_ENDPOINT}
    tls:
      insecure: true

service:
  pipelines:
    traces:
      receivers: [otlp]
      processors: [memory_limiter, batch]
      exporters: [otlp]
    metrics:
      receivers: [otlp]
      processors: [memory_limiter, batch]
      exporters: [otlp]
    logs:
      receivers: [otlp]
      processors: [memory_limiter, batch]
      exporters: [otlp]
`;

const statusColors = {
  healthy: 'text-green-400 bg-green-400/10',
  connected: 'text-blue-400 bg-blue-400/10',
  unhealthy: 'text-red-400 bg-red-400/10',
  disconnected: 'text-gray-400 bg-gray-400/10',
  pending: 'text-yellow-400 bg-yellow-400/10',
  active: 'text-green-400 bg-green-400/10',
  draft: 'text-yellow-400 bg-yellow-400/10',
  archived: 'text-gray-400 bg-gray-400/10',
};

const StatusBadge = ({ status }: { status: string }) => (
  <span className={clsx('px-2 py-1 rounded-full text-xs font-medium', statusColors[status as keyof typeof statusColors] || 'text-gray-400 bg-gray-400/10')}>
    {status}
  </span>
);

export default function ControlPlanePage() {
  const [activeTab, setActiveTab] = useState<TabType>('overview');
  const [fleetStatus, setFleetStatus] = useState<FleetStatus | null>(null);
  const [agents, setAgents] = useState<Agent[]>([]);
  const [configs, setConfigs] = useState<Config[]>([]);
  const [groups, setGroups] = useState<Group[]>([]);
  const [environments, setEnvironments] = useState<Environment[]>([]);
  const [templates, setTemplates] = useState<ConfigTemplate[]>([]);
  const [loading, setLoading] = useState(true);
  const [selectedAgent, setSelectedAgent] = useState<Agent | null>(null);
  const [selectedConfig, setSelectedConfig] = useState<Config | null>(null);
  const [showConfigModal, setShowConfigModal] = useState(false);
  const [showGroupModal, setShowGroupModal] = useState(false);
  const [showEnvModal, setShowEnvModal] = useState(false);
  const [showAgentModal, setShowAgentModal] = useState(false);
  const [showPushModal, setShowPushModal] = useState(false);
  const [editingConfig, setEditingConfig] = useState<Config | null>(null);
  const [editingGroup, setEditingGroup] = useState<Group | null>(null);
  const [editingEnv, setEditingEnv] = useState<Environment | null>(null);
  const [error, setError] = useState<string | null>(null);

  const fetchData = async () => {
    setLoading(true);
    try {
      const [statusRes, agentsRes, configsRes, groupsRes, envsRes, templatesRes] = await Promise.all([
        opampApi.getFleetStatus(),
        opampApi.getAgents(),
        opampApi.getConfigs(),
        opampApi.getGroups(),
        opampApi.getEnvironments(),
        opampApi.getTemplates().catch(() => ({ templates: [] })),
      ]);
      setFleetStatus(statusRes);
      setAgents(Array.isArray(agentsRes) ? agentsRes : []);
      setConfigs(Array.isArray(configsRes) ? configsRes : []);
      setGroups(Array.isArray(groupsRes) ? groupsRes : []);
      setEnvironments(Array.isArray(envsRes) ? envsRes : []);
      setTemplates(templatesRes.templates || []);
      setError(null);
    } catch (err) {
      console.error('Failed to fetch control plane data:', err);
      setError('Failed to connect to OpAMP server');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchData();
    const interval = setInterval(fetchData, 10000);
    return () => clearInterval(interval);
  }, []);

  const tabs = [
    { id: 'overview', name: 'Overview', icon: Activity },
    { id: 'agents', name: 'Agents', icon: Server, count: fleetStatus?.agents.total },
    { id: 'configs', name: 'Configurations', icon: Settings2, count: fleetStatus?.configs },
    { id: 'groups', name: 'Groups', icon: Layers, count: fleetStatus?.groups },
    { id: 'environments', name: 'Environments', icon: Globe, count: fleetStatus?.environments },
  ];

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold">Control Plane</h1>
          <p className="text-gray-400 mt-1">Manage your OpenTelemetry collectors and configurations</p>
        </div>
        <button
          onClick={fetchData}
          className="flex items-center gap-2 px-4 py-2 bg-gray-700 hover:bg-gray-600 rounded-lg transition-colors"
        >
          <RefreshCw className={clsx('w-4 h-4', loading && 'animate-spin')} />
          Refresh
        </button>
      </div>

      {/* Error Banner */}
      {error && (
        <div className="bg-red-500/10 border border-red-500/30 rounded-lg p-4 flex items-center gap-3">
          <XCircle className="w-5 h-5 text-red-400" />
          <span className="text-red-400">{error}</span>
        </div>
      )}

      {/* Tabs */}
      <div className="flex gap-2 border-b border-gray-700 pb-4">
        {tabs.map((tab) => (
          <button
            key={tab.id}
            onClick={() => setActiveTab(tab.id as TabType)}
            className={clsx(
              'flex items-center gap-2 px-4 py-2 rounded-lg transition-all',
              activeTab === tab.id
                ? 'bg-blue-600 text-white'
                : 'text-gray-400 hover:bg-gray-800 hover:text-white'
            )}
          >
            <tab.icon className="w-4 h-4" />
            {tab.name}
            {tab.count !== undefined && (
              <span className={clsx(
                'px-2 py-0.5 text-xs rounded-full',
                activeTab === tab.id ? 'bg-blue-500' : 'bg-gray-700'
              )}>
                {tab.count}
              </span>
            )}
          </button>
        ))}
      </div>

      {/* Content */}
      {activeTab === 'overview' && (
        <OverviewTab
          fleetStatus={fleetStatus}
          agents={agents}
          configs={configs}
          groups={groups}
        />
      )}
      {activeTab === 'agents' && (
        <AgentsTab
          agents={agents}
          groups={groups}
          configs={configs}
          selectedAgent={selectedAgent}
          onSelectAgent={setSelectedAgent}
          onRefresh={fetchData}
          onShowRegister={() => setShowAgentModal(true)}
        />
      )}
      {activeTab === 'configs' && (
        <ConfigsTab
          configs={configs}
          groups={groups}
          templates={templates}
          selectedConfig={selectedConfig}
          onSelectConfig={setSelectedConfig}
          onRefresh={fetchData}
          showModal={showConfigModal}
          setShowModal={setShowConfigModal}
          editingConfig={editingConfig}
          setEditingConfig={setEditingConfig}
          onShowPush={(config) => { setSelectedConfig(config); setShowPushModal(true); }}
        />
      )}
      {activeTab === 'groups' && (
        <GroupsTab
          groups={groups}
          environments={environments}
          configs={configs}
          agents={agents}
          onRefresh={fetchData}
          showModal={showGroupModal}
          setShowModal={setShowGroupModal}
          editingGroup={editingGroup}
          setEditingGroup={setEditingGroup}
        />
      )}
      {activeTab === 'environments' && (
        <EnvironmentsTab
          environments={environments}
          groups={groups}
          onRefresh={fetchData}
          showModal={showEnvModal}
          setShowModal={setShowEnvModal}
          editingEnv={editingEnv}
          setEditingEnv={setEditingEnv}
        />
      )}

      {/* Agent Registration Modal */}
      <AgentRegisterModal
        isOpen={showAgentModal}
        onClose={() => setShowAgentModal(false)}
        groups={groups}
        onRefresh={fetchData}
      />

      {/* Push Config Modal */}
      <PushConfigModal
        isOpen={showPushModal}
        onClose={() => setShowPushModal(false)}
        config={selectedConfig}
        groups={groups}
        agents={agents}
        environments={environments}
        onRefresh={fetchData}
      />
    </div>
  );
}

// Overview Tab Component
function OverviewTab({
  fleetStatus,
  agents,
  configs,
  groups,
}: {
  fleetStatus: FleetStatus | null;
  agents: Agent[];
  configs: Config[];
  groups: Group[];
}) {
  const stats = [
    {
      name: 'Total Agents',
      value: fleetStatus?.agents.total || 0,
      icon: Server,
      color: 'blue',
      subStats: [
        { label: 'Healthy', value: fleetStatus?.agents.healthy || 0, color: 'green' },
        { label: 'Unhealthy', value: fleetStatus?.agents.unhealthy || 0, color: 'red' },
        { label: 'Pending', value: fleetStatus?.agents.pending || 0, color: 'yellow' },
      ],
    },
    {
      name: 'Active Configs',
      value: configs.filter(c => c.status === 'active').length,
      icon: Settings2,
      color: 'purple',
      subStats: [
        { label: 'Draft', value: configs.filter(c => c.status === 'draft').length, color: 'yellow' },
        { label: 'Archived', value: configs.filter(c => c.status === 'archived').length, color: 'gray' },
      ],
    },
    {
      name: 'Groups',
      value: fleetStatus?.groups || 0,
      icon: Layers,
      color: 'cyan',
    },
    {
      name: 'Environments',
      value: fleetStatus?.environments || 0,
      icon: Globe,
      color: 'green',
    },
  ];

  const colorMap = {
    blue: 'from-blue-600 to-blue-400',
    purple: 'from-purple-600 to-purple-400',
    cyan: 'from-cyan-600 to-cyan-400',
    green: 'from-green-600 to-green-400',
  };

  return (
    <div className="space-y-6">
      {/* Stats Grid */}
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-6">
        {stats.map((stat) => (
          <div key={stat.name} className="bg-gray-800 rounded-xl p-6 border border-gray-700">
            <div className="flex items-center justify-between">
              <div className={clsx('p-3 rounded-lg bg-gradient-to-br', colorMap[stat.color as keyof typeof colorMap])}>
                <stat.icon className="w-6 h-6 text-white" />
              </div>
              <span className="text-3xl font-bold">{stat.value}</span>
            </div>
            <p className="mt-4 text-gray-400">{stat.name}</p>
            {stat.subStats && (
              <div className="mt-3 flex gap-4 text-sm">
                {stat.subStats.map((sub) => (
                  <span key={sub.label} className="flex items-center gap-1">
                    <span className={clsx(
                      'w-2 h-2 rounded-full',
                      sub.color === 'green' && 'bg-green-400',
                      sub.color === 'red' && 'bg-red-400',
                      sub.color === 'yellow' && 'bg-yellow-400',
                      sub.color === 'gray' && 'bg-gray-400'
                    )} />
                    <span className="text-gray-500">{sub.value} {sub.label}</span>
                  </span>
                ))}
              </div>
            )}
          </div>
        ))}
      </div>

      {/* Recent Activity & Agent Status */}
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        {/* Agent Health */}
        <div className="bg-gray-800 rounded-xl p-6 border border-gray-700">
          <h3 className="text-lg font-semibold mb-4">Agent Health</h3>
          <div className="space-y-3">
            {agents.slice(0, 5).map((agent) => (
              <div key={agent.id} className="flex items-center justify-between p-3 bg-gray-750 rounded-lg">
                <div className="flex items-center gap-3">
                  {agent.status === 'healthy' ? (
                    <Wifi className="w-5 h-5 text-green-400" />
                  ) : agent.status === 'connected' ? (
                    <Wifi className="w-5 h-5 text-blue-400" />
                  ) : (
                    <WifiOff className="w-5 h-5 text-gray-400" />
                  )}
                  <div>
                    <p className="font-medium">{agent.hostname}</p>
                    <p className="text-sm text-gray-400">{agent.id}</p>
                  </div>
                </div>
                <StatusBadge status={agent.status} />
              </div>
            ))}
            {agents.length === 0 && (
              <p className="text-gray-500 text-center py-4">No agents registered</p>
            )}
          </div>
        </div>

        {/* Config Versions */}
        <div className="bg-gray-800 rounded-xl p-6 border border-gray-700">
          <h3 className="text-lg font-semibold mb-4">Configuration Versions</h3>
          <div className="space-y-3">
            {configs.slice(0, 5).map((config) => (
              <div key={config.id} className="flex items-center justify-between p-3 bg-gray-750 rounded-lg">
                <div className="flex items-center gap-3">
                  <div className={clsx(
                    'p-2 rounded-lg',
                    config.status === 'active' ? 'bg-green-400/10' : 'bg-gray-700'
                  )}>
                    <Terminal className={clsx(
                      'w-5 h-5',
                      config.status === 'active' ? 'text-green-400' : 'text-gray-400'
                    )} />
                  </div>
                  <div>
                    <p className="font-medium">{config.name}</p>
                    <p className="text-sm text-gray-400">v{config.version} • {config.config_hash}</p>
                  </div>
                </div>
                <StatusBadge status={config.status} />
              </div>
            ))}
            {configs.length === 0 && (
              <p className="text-gray-500 text-center py-4">No configurations created</p>
            )}
          </div>
        </div>
      </div>

      {/* Architecture Diagram */}
      <div className="bg-gray-800 rounded-xl p-6 border border-gray-700">
        <h3 className="text-lg font-semibold mb-4">Architecture Overview</h3>
        <div className="grid grid-cols-3 gap-8">
          {/* Control Plane */}
          <div className="text-center">
            <div className="p-4 bg-gradient-to-br from-purple-600/20 to-blue-600/20 rounded-xl border border-purple-500/30">
              <Cpu className="w-12 h-12 mx-auto text-purple-400" />
              <h4 className="mt-3 font-semibold">Control Plane</h4>
              <p className="text-sm text-gray-400 mt-1">OpAMP Server</p>
              <div className="mt-3 space-y-1 text-xs text-gray-500">
                <p>• Config Management</p>
                <p>• Agent Enrollment</p>
                <p>• Health Monitoring</p>
              </div>
            </div>
          </div>

          {/* Data Plane */}
          <div className="text-center">
            <div className="p-4 bg-gradient-to-br from-cyan-600/20 to-green-600/20 rounded-xl border border-cyan-500/30">
              <HardDrive className="w-12 h-12 mx-auto text-cyan-400" />
              <h4 className="mt-3 font-semibold">Data Plane</h4>
              <p className="text-sm text-gray-400 mt-1">OTel Collectors</p>
              <div className="mt-3 space-y-1 text-xs text-gray-500">
                <p>• {fleetStatus?.agents.total || 0} Collectors</p>
                <p>• Traces, Metrics, Logs</p>
                <p>• Gateway Aggregation</p>
              </div>
            </div>
          </div>

          {/* Storage */}
          <div className="text-center">
            <div className="p-4 bg-gradient-to-br from-orange-600/20 to-red-600/20 rounded-xl border border-orange-500/30">
              <Server className="w-12 h-12 mx-auto text-orange-400" />
              <h4 className="mt-3 font-semibold">Storage</h4>
              <p className="text-sm text-gray-400 mt-1">ClickHouse</p>
              <div className="mt-3 space-y-1 text-xs text-gray-500">
                <p>• Time-series Data</p>
                <p>• Full-text Search</p>
                <p>• Analytics Engine</p>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}

// Agents Tab Component
function AgentsTab({
  agents,
  groups,
  configs,
  selectedAgent,
  onSelectAgent,
  onRefresh,
  onShowRegister,
}: {
  agents: Agent[];
  groups: Group[];
  configs: Config[];
  selectedAgent: Agent | null;
  onSelectAgent: (agent: Agent | null) => void;
  onRefresh: () => void;
  onShowRegister: () => void;
}) {
  const [deleting, setDeleting] = useState<string | null>(null);

  const getGroupName = (groupId?: string) => {
    if (!groupId) return 'Unassigned';
    return groups.find(g => g.id === groupId)?.name || 'Unknown';
  };

  const handleDelete = async (agentId: string) => {
    if (!confirm('Are you sure you want to delete this agent?')) return;
    setDeleting(agentId);
    try {
      await opampApi.deleteAgent(agentId);
      onRefresh();
      if (selectedAgent?.id === agentId) onSelectAgent(null);
    } catch (error) {
      console.error('Failed to delete agent:', error);
    } finally {
      setDeleting(null);
    }
  };

  const handleAssignGroup = async (agentId: string, groupId: string) => {
    try {
      await opampApi.updateAgent(agentId, { group_id: groupId || undefined });
      onRefresh();
    } catch (error) {
      console.error('Failed to update agent:', error);
    }
  };

  return (
    <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
      {/* Agent List */}
      <div className="lg:col-span-2 bg-gray-800 rounded-xl border border-gray-700">
        <div className="p-4 border-b border-gray-700 flex items-center justify-between">
          <h3 className="font-semibold">Registered Agents ({agents.length})</h3>
          <button
            onClick={onShowRegister}
            className="flex items-center gap-2 px-3 py-1.5 bg-blue-600 hover:bg-blue-700 rounded-lg text-sm transition-colors"
          >
            <Plus className="w-4 h-4" />
            Register Agent
          </button>
        </div>
        <div className="divide-y divide-gray-700">
          {agents.map((agent) => (
            <div
              key={agent.id}
              onClick={() => onSelectAgent(agent)}
              className={clsx(
                'p-4 cursor-pointer transition-colors',
                selectedAgent?.id === agent.id ? 'bg-gray-700' : 'hover:bg-gray-750'
              )}
            >
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-3">
                  <div className={clsx(
                    'p-2 rounded-lg',
                    agent.status === 'healthy' ? 'bg-green-400/10' :
                    agent.status === 'connected' ? 'bg-blue-400/10' : 'bg-gray-700'
                  )}>
                    <Server className={clsx(
                      'w-5 h-5',
                      agent.status === 'healthy' ? 'text-green-400' :
                      agent.status === 'connected' ? 'text-blue-400' : 'text-gray-400'
                    )} />
                  </div>
                  <div>
                    <p className="font-medium">{agent.hostname}</p>
                    <p className="text-sm text-gray-400">{agent.ip_address || 'No IP'}</p>
                  </div>
                </div>
                <div className="flex items-center gap-3">
                  <span className="text-sm text-gray-400">{getGroupName(agent.group_id)}</span>
                  <StatusBadge status={agent.status} />
                  <ChevronRight className="w-4 h-4 text-gray-500" />
                </div>
              </div>
            </div>
          ))}
          {agents.length === 0 && (
            <div className="p-8 text-center text-gray-500">
              <Server className="w-12 h-12 mx-auto mb-3 opacity-50" />
              <p>No agents registered yet</p>
              <p className="text-sm mt-1">Register an agent to get started</p>
            </div>
          )}
        </div>
      </div>

      {/* Agent Details */}
      <div className="bg-gray-800 rounded-xl border border-gray-700">
        <div className="p-4 border-b border-gray-700">
          <h3 className="font-semibold">Agent Details</h3>
        </div>
        {selectedAgent ? (
          <div className="p-4 space-y-4">
            <div>
              <label className="text-sm text-gray-400">Agent ID</label>
              <p className="font-mono text-sm mt-1">{selectedAgent.id}</p>
            </div>
            <div>
              <label className="text-sm text-gray-400">Hostname</label>
              <p className="mt-1">{selectedAgent.hostname}</p>
            </div>
            <div>
              <label className="text-sm text-gray-400">IP Address</label>
              <p className="mt-1">{selectedAgent.ip_address || 'N/A'}</p>
            </div>
            <div>
              <label className="text-sm text-gray-400">Status</label>
              <div className="mt-1">
                <StatusBadge status={selectedAgent.status} />
              </div>
            </div>
            <div>
              <label className="text-sm text-gray-400">Group</label>
              <select
                value={selectedAgent.group_id || ''}
                onChange={(e) => handleAssignGroup(selectedAgent.id, e.target.value)}
                className="w-full mt-1 px-3 py-2 bg-gray-700 border border-gray-600 rounded-lg focus:outline-none focus:border-blue-500 text-sm"
              >
                <option value="">Unassigned</option>
                {groups.map((group) => (
                  <option key={group.id} value={group.id}>{group.name}</option>
                ))}
              </select>
            </div>
            <div>
              <label className="text-sm text-gray-400">Capabilities</label>
              <div className="flex flex-wrap gap-2 mt-1">
                {(selectedAgent.capabilities || []).map((cap) => (
                  <span key={cap} className="px-2 py-1 bg-gray-700 rounded text-xs">
                    {cap}
                  </span>
                ))}
              </div>
            </div>
            <div>
              <label className="text-sm text-gray-400">Config Hash</label>
              <p className="font-mono text-sm mt-1">{selectedAgent.effective_config_hash || 'None'}</p>
            </div>
            <div>
              <label className="text-sm text-gray-400">Last Seen</label>
              <p className="mt-1">{selectedAgent.last_seen ? new Date(selectedAgent.last_seen).toLocaleString() : 'Never'}</p>
            </div>
            {selectedAgent.labels && Object.keys(selectedAgent.labels).length > 0 && (
              <div>
                <label className="text-sm text-gray-400">Labels</label>
                <div className="flex flex-wrap gap-2 mt-1">
                  {Object.entries(selectedAgent.labels).map(([key, value]) => (
                    <span key={key} className="px-2 py-1 bg-gray-700 rounded text-xs">
                      {key}: {value}
                    </span>
                  ))}
                </div>
              </div>
            )}
            {selectedAgent.last_error && (
              <div>
                <label className="text-sm text-gray-400">Last Error</label>
                <p className="mt-1 text-red-400 text-sm">{selectedAgent.last_error}</p>
              </div>
            )}
            <div className="pt-4">
              <button
                onClick={() => handleDelete(selectedAgent.id)}
                disabled={deleting === selectedAgent.id}
                className="w-full px-3 py-2 bg-red-600/10 hover:bg-red-600/20 text-red-400 rounded-lg text-sm transition-colors flex items-center justify-center gap-2"
              >
                <Trash2 className="w-4 h-4" />
                {deleting === selectedAgent.id ? 'Deleting...' : 'Delete Agent'}
              </button>
            </div>
          </div>
        ) : (
          <div className="p-8 text-center text-gray-500">
            <Server className="w-12 h-12 mx-auto mb-3 opacity-50" />
            <p>Select an agent to view details</p>
          </div>
        )}
      </div>
    </div>
  );
}

// Configs Tab Component
function ConfigsTab({
  configs,
  groups,
  templates,
  selectedConfig,
  onSelectConfig,
  onRefresh,
  showModal,
  setShowModal,
  editingConfig,
  setEditingConfig,
  onShowPush,
}: {
  configs: Config[];
  groups: Group[];
  templates: ConfigTemplate[];
  selectedConfig: Config | null;
  onSelectConfig: (config: Config | null) => void;
  onRefresh: () => void;
  showModal: boolean;
  setShowModal: (show: boolean) => void;
  editingConfig: Config | null;
  setEditingConfig: (config: Config | null) => void;
  onShowPush: (config: Config) => void;
}) {
  const [formName, setFormName] = useState('');
  const [formDescription, setFormDescription] = useState('');
  const [formYaml, setFormYaml] = useState('');
  const [formLabels, setFormLabels] = useState('');
  const [saving, setSaving] = useState(false);
  const [deleting, setDeleting] = useState<string | null>(null);

  const handleActivate = async (configId: string) => {
    try {
      await opampApi.activateConfig(configId);
      onRefresh();
    } catch (error) {
      console.error('Failed to activate config:', error);
    }
  };

  const handleDelete = async (configId: string) => {
    if (!confirm('Are you sure you want to delete this configuration?')) return;
    setDeleting(configId);
    try {
      await opampApi.deleteConfig(configId);
      onRefresh();
      if (selectedConfig?.id === configId) onSelectConfig(null);
    } catch (error) {
      console.error('Failed to delete config:', error);
    } finally {
      setDeleting(null);
    }
  };

  const openCreateModal = () => {
    setFormName('');
    setFormDescription('');
    setFormYaml(defaultOtelConfig);
    setFormLabels('');
    setEditingConfig(null);
    setShowModal(true);
  };

  const openEditModal = (config: Config) => {
    setFormName(config.name);
    setFormDescription(config.description || '');
    setFormYaml(config.config_yaml);
    setFormLabels(Object.entries(config.labels || {}).map(([k, v]) => `${k}=${v}`).join(', '));
    setEditingConfig(config);
    setShowModal(true);
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setSaving(true);
    try {
      const labelObj: Record<string, string> = {};
      if (formLabels) {
        formLabels.split(',').forEach(pair => {
          const [key, value] = pair.split('=').map(s => s.trim());
          if (key && value) labelObj[key] = value;
        });
      }

      if (editingConfig) {
        await opampApi.updateConfig(editingConfig.id, {
          name: formName,
          description: formDescription,
          config_yaml: formYaml,
          labels: labelObj,
        });
      } else {
        await opampApi.createConfig({
          name: formName,
          description: formDescription,
          config_yaml: formYaml,
          labels: labelObj,
        });
      }
      setShowModal(false);
      onRefresh();
    } catch (error) {
      console.error('Failed to save config:', error);
    } finally {
      setSaving(false);
    }
  };

  const loadTemplate = (template: ConfigTemplate) => {
    setFormName(template.name);
    setFormDescription(template.description);
    setFormYaml(template.config_yaml);
  };

  return (
    <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
      {/* Config List */}
      <div className="bg-gray-800 rounded-xl border border-gray-700">
        <div className="p-4 border-b border-gray-700 flex items-center justify-between">
          <h3 className="font-semibold">Configurations ({configs.length})</h3>
          <button
            onClick={openCreateModal}
            className="flex items-center gap-2 px-3 py-1.5 bg-blue-600 hover:bg-blue-700 rounded-lg text-sm transition-colors"
          >
            <Plus className="w-4 h-4" />
            New Config
          </button>
        </div>
        <div className="divide-y divide-gray-700">
          {configs.map((config) => (
            <div
              key={config.id}
              onClick={() => onSelectConfig(config)}
              className={clsx(
                'p-4 cursor-pointer transition-colors',
                selectedConfig?.id === config.id ? 'bg-gray-700' : 'hover:bg-gray-750'
              )}
            >
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-3">
                  <div className={clsx(
                    'p-2 rounded-lg',
                    config.status === 'active' ? 'bg-green-400/10' : 'bg-gray-700'
                  )}>
                    <Settings2 className={clsx(
                      'w-5 h-5',
                      config.status === 'active' ? 'text-green-400' : 'text-gray-400'
                    )} />
                  </div>
                  <div>
                    <p className="font-medium">{config.name}</p>
                    <p className="text-sm text-gray-400">
                      v{config.version} • {config.config_hash.slice(0, 8)}
                    </p>
                  </div>
                </div>
                <div className="flex items-center gap-3">
                  <StatusBadge status={config.status} />
                  {config.status !== 'active' && (
                    <button
                      onClick={(e) => {
                        e.stopPropagation();
                        handleActivate(config.id);
                      }}
                      className="p-1.5 hover:bg-gray-600 rounded transition-colors"
                      title="Activate"
                    >
                      <Play className="w-4 h-4 text-green-400" />
                    </button>
                  )}
                </div>
              </div>
            </div>
          ))}
          {configs.length === 0 && (
            <div className="p-8 text-center text-gray-500">
              <Settings2 className="w-12 h-12 mx-auto mb-3 opacity-50" />
              <p>No configurations created yet</p>
            </div>
          )}
        </div>
      </div>

      {/* Config Preview */}
      <div className="bg-gray-800 rounded-xl border border-gray-700">
        <div className="p-4 border-b border-gray-700 flex items-center justify-between">
          <h3 className="font-semibold">Configuration Preview</h3>
          {selectedConfig && (
            <div className="flex gap-2">
              <button
                onClick={() => openEditModal(selectedConfig)}
                className="px-3 py-1.5 bg-gray-700 hover:bg-gray-600 rounded-lg text-sm transition-colors flex items-center gap-1"
              >
                <Edit className="w-3 h-3" />
                Edit
              </button>
              <button
                onClick={() => onShowPush(selectedConfig)}
                className="px-3 py-1.5 bg-blue-600 hover:bg-blue-700 rounded-lg text-sm transition-colors flex items-center gap-1"
              >
                <Upload className="w-3 h-3" />
                Push
              </button>
              <button
                onClick={() => handleDelete(selectedConfig.id)}
                disabled={deleting === selectedConfig.id}
                className="px-3 py-1.5 bg-red-600/10 hover:bg-red-600/20 text-red-400 rounded-lg text-sm transition-colors"
              >
                <Trash2 className="w-4 h-4" />
              </button>
            </div>
          )}
        </div>
        {selectedConfig ? (
          <div className="p-4">
            <div className="mb-4 flex items-center justify-between">
              <div>
                <h4 className="font-medium">{selectedConfig.name}</h4>
                <p className="text-sm text-gray-400">{selectedConfig.description}</p>
              </div>
              <div className="flex items-center gap-2">
                <StatusBadge status={selectedConfig.status} />
                {selectedConfig.status !== 'active' && (
                  <button
                    onClick={() => handleActivate(selectedConfig.id)}
                    className="px-2 py-1 bg-green-600/10 hover:bg-green-600/20 text-green-400 rounded text-xs flex items-center gap-1"
                  >
                    <Play className="w-3 h-3" />
                    Activate
                  </button>
                )}
              </div>
            </div>
            <pre className="p-4 bg-gray-900 rounded-lg overflow-auto text-sm font-mono text-gray-300 max-h-96">
              {selectedConfig.config_yaml}
            </pre>
            <div className="mt-4 flex flex-wrap gap-2">
              {Object.entries(selectedConfig.labels || {}).map(([key, value]) => (
                <span key={key} className="px-2 py-1 bg-gray-700 rounded text-xs">
                  {key}: {value}
                </span>
              ))}
            </div>
          </div>
        ) : (
          <div className="p-8 text-center text-gray-500">
            <Terminal className="w-12 h-12 mx-auto mb-3 opacity-50" />
            <p>Select a configuration to preview</p>
          </div>
        )}
      </div>

      {/* Create/Edit Config Modal */}
      <Modal isOpen={showModal} onClose={() => setShowModal(false)} title={editingConfig ? 'Edit Configuration' : 'Create Configuration'}>
        <form onSubmit={handleSubmit} className="space-y-4">
          {!editingConfig && templates.length > 0 && (
            <div>
              <label className="block text-sm font-medium mb-2">Load from Template</label>
              <div className="flex flex-wrap gap-2">
                {templates.map((t) => (
                  <button
                    key={t.id}
                    type="button"
                    onClick={() => loadTemplate(t)}
                    className="px-3 py-1.5 bg-gray-700 hover:bg-gray-600 rounded-lg text-sm transition-colors"
                  >
                    {t.name}
                  </button>
                ))}
              </div>
            </div>
          )}
          <div>
            <label className="block text-sm font-medium mb-2">Name *</label>
            <input
              type="text"
              value={formName}
              onChange={(e) => setFormName(e.target.value)}
              placeholder="e.g., production-collector"
              className="w-full px-4 py-2 bg-gray-700 border border-gray-600 rounded-lg focus:outline-none focus:border-blue-500"
              required
            />
          </div>
          <div>
            <label className="block text-sm font-medium mb-2">Description</label>
            <input
              type="text"
              value={formDescription}
              onChange={(e) => setFormDescription(e.target.value)}
              placeholder="e.g., Configuration for production collectors"
              className="w-full px-4 py-2 bg-gray-700 border border-gray-600 rounded-lg focus:outline-none focus:border-blue-500"
            />
          </div>
          <div>
            <label className="block text-sm font-medium mb-2">Configuration YAML *</label>
            <textarea
              value={formYaml}
              onChange={(e) => setFormYaml(e.target.value)}
              placeholder="OpenTelemetry Collector configuration..."
              rows={15}
              className="w-full px-4 py-2 bg-gray-700 border border-gray-600 rounded-lg focus:outline-none focus:border-blue-500 font-mono text-sm"
              required
            />
          </div>
          <div>
            <label className="block text-sm font-medium mb-2">Labels</label>
            <input
              type="text"
              value={formLabels}
              onChange={(e) => setFormLabels(e.target.value)}
              placeholder="e.g., env=prod, team=platform"
              className="w-full px-4 py-2 bg-gray-700 border border-gray-600 rounded-lg focus:outline-none focus:border-blue-500"
            />
            <p className="text-xs text-gray-500 mt-1">Comma-separated key=value pairs</p>
          </div>
          <div className="flex gap-3 pt-4">
            <button
              type="button"
              onClick={() => setShowModal(false)}
              className="flex-1 py-2 bg-gray-700 hover:bg-gray-600 rounded-lg transition-colors"
            >
              Cancel
            </button>
            <button
              type="submit"
              disabled={saving || !formName || !formYaml}
              className="flex-1 py-2 bg-blue-600 hover:bg-blue-700 disabled:opacity-50 rounded-lg transition-colors"
            >
              {saving ? 'Saving...' : editingConfig ? 'Update' : 'Create'}
            </button>
          </div>
        </form>
      </Modal>
    </div>
  );
}

// Groups Tab Component
function GroupsTab({
  groups,
  environments,
  configs,
  agents,
  onRefresh,
  showModal,
  setShowModal,
  editingGroup,
  setEditingGroup,
}: {
  groups: Group[];
  environments: Environment[];
  configs: Config[];
  agents: Agent[];
  onRefresh: () => void;
  showModal: boolean;
  setShowModal: (show: boolean) => void;
  editingGroup: Group | null;
  setEditingGroup: (group: Group | null) => void;
}) {
  const [formName, setFormName] = useState('');
  const [formDescription, setFormDescription] = useState('');
  const [formEnvId, setFormEnvId] = useState('');
  const [formConfigId, setFormConfigId] = useState('');
  const [formLabels, setFormLabels] = useState('');
  const [saving, setSaving] = useState(false);
  const [deleting, setDeleting] = useState<string | null>(null);

  const getEnvName = (envId?: string) => {
    if (!envId) return 'No Environment';
    return environments.find(e => e.id === envId)?.name || 'Unknown';
  };

  const getConfigName = (configId?: string) => {
    if (!configId) return 'No Config';
    return configs.find(c => c.id === configId)?.name || 'Unknown';
  };

  const getAgentCount = (groupId: string) => {
    return agents.filter(a => a.group_id === groupId).length;
  };

  const openCreateModal = () => {
    setFormName('');
    setFormDescription('');
    setFormEnvId('');
    setFormConfigId('');
    setFormLabels('');
    setEditingGroup(null);
    setShowModal(true);
  };

  const openEditModal = (group: Group) => {
    setFormName(group.name);
    setFormDescription(group.description || '');
    setFormEnvId(group.environment_id || '');
    setFormConfigId(group.config_id || '');
    setFormLabels(Object.entries(group.labels || {}).map(([k, v]) => `${k}=${v}`).join(', '));
    setEditingGroup(group);
    setShowModal(true);
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setSaving(true);
    try {
      const labelObj: Record<string, string> = {};
      if (formLabels) {
        formLabels.split(',').forEach(pair => {
          const [key, value] = pair.split('=').map(s => s.trim());
          if (key && value) labelObj[key] = value;
        });
      }

      if (editingGroup) {
        await opampApi.updateGroup(editingGroup.id, {
          name: formName,
          description: formDescription,
          environment_id: formEnvId || undefined,
          config_id: formConfigId || undefined,
          labels: labelObj,
        });
      } else {
        await opampApi.createGroup({
          name: formName,
          description: formDescription,
          environment_id: formEnvId || undefined,
          config_id: formConfigId || undefined,
          labels: labelObj,
        });
      }
      setShowModal(false);
      onRefresh();
    } catch (error) {
      console.error('Failed to save group:', error);
    } finally {
      setSaving(false);
    }
  };

  const handleDelete = async (groupId: string) => {
    if (!confirm('Are you sure you want to delete this group?')) return;
    setDeleting(groupId);
    try {
      await opampApi.deleteGroup(groupId);
      onRefresh();
    } catch (error) {
      console.error('Failed to delete group:', error);
    } finally {
      setDeleting(null);
    }
  };

  return (
    <div className="space-y-6">
      <div className="flex justify-end">
        <button
          onClick={openCreateModal}
          className="flex items-center gap-2 px-4 py-2 bg-blue-600 hover:bg-blue-700 rounded-lg transition-colors"
        >
          <Plus className="w-4 h-4" />
          Create Group
        </button>
      </div>

      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
        {groups.map((group) => (
          <div key={group.id} className="bg-gray-800 rounded-xl border border-gray-700 p-6">
            <div className="flex items-start justify-between">
              <div className="flex items-center gap-3">
                <div className="p-3 bg-gradient-to-br from-cyan-600/20 to-blue-600/20 rounded-lg">
                  <Layers className="w-6 h-6 text-cyan-400" />
                </div>
                <div>
                  <h3 className="font-semibold">{group.name}</h3>
                  <p className="text-sm text-gray-400">{group.description || 'No description'}</p>
                </div>
              </div>
              <div className="flex gap-1">
                <button
                  onClick={() => openEditModal(group)}
                  className="p-1.5 hover:bg-gray-700 rounded transition-colors"
                >
                  <Edit className="w-4 h-4 text-gray-400" />
                </button>
                <button
                  onClick={() => handleDelete(group.id)}
                  disabled={deleting === group.id}
                  className="p-1.5 hover:bg-red-600/20 rounded transition-colors"
                >
                  <Trash2 className="w-4 h-4 text-red-400" />
                </button>
              </div>
            </div>

            <div className="mt-4 space-y-2 text-sm">
              <div className="flex items-center justify-between">
                <span className="text-gray-400">Environment</span>
                <span>{getEnvName(group.environment_id)}</span>
              </div>
              <div className="flex items-center justify-between">
                <span className="text-gray-400">Configuration</span>
                <span>{getConfigName(group.config_id)}</span>
              </div>
              <div className="flex items-center justify-between">
                <span className="text-gray-400">Agents</span>
                <span className="flex items-center gap-1">
                  <Server className="w-4 h-4 text-gray-400" />
                  {getAgentCount(group.id)}
                </span>
              </div>
            </div>

            {Object.keys(group.labels || {}).length > 0 && (
              <div className="mt-4 flex flex-wrap gap-2">
                {Object.entries(group.labels).map(([key, value]) => (
                  <span key={key} className="px-2 py-1 bg-gray-700 rounded text-xs">
                    {key}: {value}
                  </span>
                ))}
              </div>
            )}
          </div>
        ))}
        {groups.length === 0 && (
          <div className="col-span-full p-8 text-center text-gray-500 bg-gray-800 rounded-xl border border-gray-700">
            <Layers className="w-12 h-12 mx-auto mb-3 opacity-50" />
            <p>No groups created yet</p>
            <p className="text-sm mt-1">Create a group to organize your agents</p>
          </div>
        )}
      </div>

      {/* Create/Edit Group Modal */}
      <Modal isOpen={showModal} onClose={() => setShowModal(false)} title={editingGroup ? 'Edit Group' : 'Create Group'}>
        <form onSubmit={handleSubmit} className="space-y-4">
          <div>
            <label className="block text-sm font-medium mb-2">Name *</label>
            <input
              type="text"
              value={formName}
              onChange={(e) => setFormName(e.target.value)}
              placeholder="e.g., production-collectors"
              className="w-full px-4 py-2 bg-gray-700 border border-gray-600 rounded-lg focus:outline-none focus:border-blue-500"
              required
            />
          </div>
          <div>
            <label className="block text-sm font-medium mb-2">Description</label>
            <input
              type="text"
              value={formDescription}
              onChange={(e) => setFormDescription(e.target.value)}
              placeholder="e.g., Collectors in production environment"
              className="w-full px-4 py-2 bg-gray-700 border border-gray-600 rounded-lg focus:outline-none focus:border-blue-500"
            />
          </div>
          <div>
            <label className="block text-sm font-medium mb-2">Environment</label>
            <select
              value={formEnvId}
              onChange={(e) => setFormEnvId(e.target.value)}
              className="w-full px-4 py-2 bg-gray-700 border border-gray-600 rounded-lg focus:outline-none focus:border-blue-500"
            >
              <option value="">No Environment</option>
              {environments.map((env) => (
                <option key={env.id} value={env.id}>{env.name}</option>
              ))}
            </select>
          </div>
          <div>
            <label className="block text-sm font-medium mb-2">Default Configuration</label>
            <select
              value={formConfigId}
              onChange={(e) => setFormConfigId(e.target.value)}
              className="w-full px-4 py-2 bg-gray-700 border border-gray-600 rounded-lg focus:outline-none focus:border-blue-500"
            >
              <option value="">No Default Config</option>
              {configs.filter(c => c.status === 'active').map((config) => (
                <option key={config.id} value={config.id}>{config.name}</option>
              ))}
            </select>
          </div>
          <div>
            <label className="block text-sm font-medium mb-2">Labels</label>
            <input
              type="text"
              value={formLabels}
              onChange={(e) => setFormLabels(e.target.value)}
              placeholder="e.g., team=platform, tier=1"
              className="w-full px-4 py-2 bg-gray-700 border border-gray-600 rounded-lg focus:outline-none focus:border-blue-500"
            />
            <p className="text-xs text-gray-500 mt-1">Comma-separated key=value pairs</p>
          </div>
          <div className="flex gap-3 pt-4">
            <button
              type="button"
              onClick={() => setShowModal(false)}
              className="flex-1 py-2 bg-gray-700 hover:bg-gray-600 rounded-lg transition-colors"
            >
              Cancel
            </button>
            <button
              type="submit"
              disabled={saving || !formName}
              className="flex-1 py-2 bg-blue-600 hover:bg-blue-700 disabled:opacity-50 rounded-lg transition-colors"
            >
              {saving ? 'Saving...' : editingGroup ? 'Update' : 'Create'}
            </button>
          </div>
        </form>
      </Modal>
    </div>
  );
}

// Environments Tab Component
function EnvironmentsTab({
  environments,
  groups,
  onRefresh,
  showModal,
  setShowModal,
  editingEnv,
  setEditingEnv,
}: {
  environments: Environment[];
  groups: Group[];
  onRefresh: () => void;
  showModal: boolean;
  setShowModal: (show: boolean) => void;
  editingEnv: Environment | null;
  setEditingEnv: (env: Environment | null) => void;
}) {
  const [formName, setFormName] = useState('');
  const [formDescription, setFormDescription] = useState('');
  const [formVariables, setFormVariables] = useState('');
  const [saving, setSaving] = useState(false);
  const [deleting, setDeleting] = useState<string | null>(null);

  const getGroupCount = (envId: string) => {
    return groups.filter(g => g.environment_id === envId).length;
  };

  const openCreateModal = () => {
    setFormName('');
    setFormDescription('');
    setFormVariables('OTEL_EXPORTER_ENDPOINT=http://collector:4317');
    setEditingEnv(null);
    setShowModal(true);
  };

  const openEditModal = (env: Environment) => {
    setFormName(env.name);
    setFormDescription(env.description || '');
    setFormVariables(Object.entries(env.variables || {}).map(([k, v]) => `${k}=${v}`).join('\n'));
    setEditingEnv(env);
    setShowModal(true);
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setSaving(true);
    try {
      const variables: Record<string, string> = {};
      if (formVariables) {
        formVariables.split('\n').forEach(line => {
          const idx = line.indexOf('=');
          if (idx > 0) {
            const key = line.slice(0, idx).trim();
            const value = line.slice(idx + 1).trim();
            if (key) variables[key] = value;
          }
        });
      }

      if (editingEnv) {
        await opampApi.updateEnvironment(editingEnv.id, {
          name: formName,
          description: formDescription,
          variables,
        });
      } else {
        await opampApi.createEnvironment({
          name: formName,
          description: formDescription,
          variables,
        });
      }
      setShowModal(false);
      onRefresh();
    } catch (error) {
      console.error('Failed to save environment:', error);
    } finally {
      setSaving(false);
    }
  };

  const handleDelete = async (envId: string) => {
    if (!confirm('Are you sure you want to delete this environment?')) return;
    setDeleting(envId);
    try {
      await opampApi.deleteEnvironment(envId);
      onRefresh();
    } catch (error) {
      console.error('Failed to delete environment:', error);
    } finally {
      setDeleting(null);
    }
  };

  return (
    <div className="space-y-6">
      <div className="flex justify-end">
        <button
          onClick={openCreateModal}
          className="flex items-center gap-2 px-4 py-2 bg-blue-600 hover:bg-blue-700 rounded-lg transition-colors"
        >
          <Plus className="w-4 h-4" />
          Create Environment
        </button>
      </div>

      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
        {environments.map((env) => (
          <div key={env.id} className="bg-gray-800 rounded-xl border border-gray-700 p-6">
            <div className="flex items-start justify-between">
              <div className="flex items-center gap-3">
                <div className="p-3 bg-gradient-to-br from-green-600/20 to-emerald-600/20 rounded-lg">
                  <Globe className="w-6 h-6 text-green-400" />
                </div>
                <div>
                  <h3 className="font-semibold">{env.name}</h3>
                  <p className="text-sm text-gray-400">{env.description || 'No description'}</p>
                </div>
              </div>
              <div className="flex gap-1">
                <button
                  onClick={() => openEditModal(env)}
                  className="p-1.5 hover:bg-gray-700 rounded transition-colors"
                >
                  <Edit className="w-4 h-4 text-gray-400" />
                </button>
                <button
                  onClick={() => handleDelete(env.id)}
                  disabled={deleting === env.id}
                  className="p-1.5 hover:bg-red-600/20 rounded transition-colors"
                >
                  <Trash2 className="w-4 h-4 text-red-400" />
                </button>
              </div>
            </div>

            <div className="mt-4 space-y-2 text-sm">
              <div className="flex items-center justify-between">
                <span className="text-gray-400">Groups</span>
                <span className="flex items-center gap-1">
                  <Layers className="w-4 h-4 text-gray-400" />
                  {getGroupCount(env.id)}
                </span>
              </div>
            </div>

            {Object.keys(env.variables || {}).length > 0 && (
              <div className="mt-4">
                <p className="text-sm text-gray-400 mb-2">Environment Variables ({Object.keys(env.variables).length})</p>
                <div className="space-y-1">
                  {Object.entries(env.variables).slice(0, 3).map(([key]) => (
                    <div key={key} className="flex items-center justify-between text-sm p-2 bg-gray-750 rounded">
                      <span className="font-mono text-gray-300 truncate">{key}</span>
                      <span className="font-mono text-gray-500">••••••</span>
                    </div>
                  ))}
                  {Object.keys(env.variables).length > 3 && (
                    <p className="text-xs text-gray-500 text-center">+{Object.keys(env.variables).length - 3} more</p>
                  )}
                </div>
              </div>
            )}
          </div>
        ))}
        {environments.length === 0 && (
          <div className="col-span-full p-8 text-center text-gray-500 bg-gray-800 rounded-xl border border-gray-700">
            <Globe className="w-12 h-12 mx-auto mb-3 opacity-50" />
            <p>No environments created yet</p>
            <p className="text-sm mt-1">Create an environment to organize your infrastructure</p>
          </div>
        )}
      </div>

      {/* Create/Edit Environment Modal */}
      <Modal isOpen={showModal} onClose={() => setShowModal(false)} title={editingEnv ? 'Edit Environment' : 'Create Environment'}>
        <form onSubmit={handleSubmit} className="space-y-4">
          <div>
            <label className="block text-sm font-medium mb-2">Name *</label>
            <input
              type="text"
              value={formName}
              onChange={(e) => setFormName(e.target.value)}
              placeholder="e.g., production"
              className="w-full px-4 py-2 bg-gray-700 border border-gray-600 rounded-lg focus:outline-none focus:border-blue-500"
              required
            />
          </div>
          <div>
            <label className="block text-sm font-medium mb-2">Description</label>
            <input
              type="text"
              value={formDescription}
              onChange={(e) => setFormDescription(e.target.value)}
              placeholder="e.g., Production environment"
              className="w-full px-4 py-2 bg-gray-700 border border-gray-600 rounded-lg focus:outline-none focus:border-blue-500"
            />
          </div>
          <div>
            <label className="block text-sm font-medium mb-2">Environment Variables</label>
            <textarea
              value={formVariables}
              onChange={(e) => setFormVariables(e.target.value)}
              placeholder="KEY=value (one per line)"
              rows={6}
              className="w-full px-4 py-2 bg-gray-700 border border-gray-600 rounded-lg focus:outline-none focus:border-blue-500 font-mono text-sm"
            />
            <p className="text-xs text-gray-500 mt-1">One variable per line in KEY=value format. These will be available for config templating.</p>
          </div>
          <div className="flex gap-3 pt-4">
            <button
              type="button"
              onClick={() => setShowModal(false)}
              className="flex-1 py-2 bg-gray-700 hover:bg-gray-600 rounded-lg transition-colors"
            >
              Cancel
            </button>
            <button
              type="submit"
              disabled={saving || !formName}
              className="flex-1 py-2 bg-blue-600 hover:bg-blue-700 disabled:opacity-50 rounded-lg transition-colors"
            >
              {saving ? 'Saving...' : editingEnv ? 'Update' : 'Create'}
            </button>
          </div>
        </form>
      </Modal>
    </div>
  );
}

// Agent Registration Modal
function AgentRegisterModal({
  isOpen,
  onClose,
  groups,
  onRefresh,
}: {
  isOpen: boolean;
  onClose: () => void;
  groups: Group[];
  onRefresh: () => void;
}) {
  const [hostname, setHostname] = useState('');
  const [ipAddress, setIpAddress] = useState('');
  const [groupId, setGroupId] = useState('');
  const [labels, setLabels] = useState('');
  const [saving, setSaving] = useState(false);
  const [enrollmentCommand, setEnrollmentCommand] = useState('');

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setSaving(true);
    try {
      const labelObj: Record<string, string> = {};
      if (labels) {
        labels.split(',').forEach(pair => {
          const [key, value] = pair.split('=').map(s => s.trim());
          if (key && value) labelObj[key] = value;
        });
      }

      const agent = await opampApi.registerAgent({
        hostname,
        ip_address: ipAddress || undefined,
        group_id: groupId || undefined,
        labels: Object.keys(labelObj).length > 0 ? labelObj : undefined,
        capabilities: ['AcceptsRemoteConfig', 'ReportsStatus', 'ReportsHealth'],
      });

      // Generate enrollment command
      const cmd = `# Install OpenTelemetry Collector with OpAMP support
curl -fsSL https://github.com/open-telemetry/opentelemetry-collector-releases/releases/download/v0.92.0/otelcol-contrib_0.92.0_linux_amd64.tar.gz | tar xz
./otelcol-contrib --config=<your-config.yaml>

# Or use Docker:
docker run -d --name otel-collector \\
  -e OPAMP_ENDPOINT=wss://ollystack.com/opamp/v1/opamp \\
  -e AGENT_ID=${agent.id} \\
  otel/opentelemetry-collector-contrib:latest`;

      setEnrollmentCommand(cmd);
      onRefresh();
    } catch (error) {
      console.error('Failed to register agent:', error);
    } finally {
      setSaving(false);
    }
  };

  const handleClose = () => {
    setHostname('');
    setIpAddress('');
    setGroupId('');
    setLabels('');
    setEnrollmentCommand('');
    onClose();
  };

  return (
    <Modal isOpen={isOpen} onClose={handleClose} title="Register Agent">
      {enrollmentCommand ? (
        <div className="space-y-4">
          <div className="flex items-center gap-2 text-green-400">
            <CheckCircle className="w-5 h-5" />
            <span>Agent registered successfully!</span>
          </div>
          <div>
            <label className="block text-sm text-gray-400 mb-2">Enrollment Command</label>
            <div className="relative">
              <pre className="p-4 bg-gray-900 rounded-lg text-sm font-mono text-gray-300 overflow-x-auto">
                {enrollmentCommand}
              </pre>
              <button
                onClick={() => navigator.clipboard.writeText(enrollmentCommand)}
                className="absolute top-2 right-2 p-2 bg-gray-700 hover:bg-gray-600 rounded-lg transition-colors"
              >
                <Copy className="w-4 h-4" />
              </button>
            </div>
          </div>
          <button
            onClick={handleClose}
            className="w-full py-2 bg-blue-600 hover:bg-blue-700 rounded-lg transition-colors"
          >
            Done
          </button>
        </div>
      ) : (
        <form onSubmit={handleSubmit} className="space-y-4">
          <div>
            <label className="block text-sm font-medium mb-2">Hostname *</label>
            <input
              type="text"
              value={hostname}
              onChange={(e) => setHostname(e.target.value)}
              placeholder="e.g., collector-prod-01"
              className="w-full px-4 py-2 bg-gray-700 border border-gray-600 rounded-lg focus:outline-none focus:border-blue-500"
              required
            />
          </div>
          <div>
            <label className="block text-sm font-medium mb-2">IP Address</label>
            <input
              type="text"
              value={ipAddress}
              onChange={(e) => setIpAddress(e.target.value)}
              placeholder="e.g., 10.0.1.50"
              className="w-full px-4 py-2 bg-gray-700 border border-gray-600 rounded-lg focus:outline-none focus:border-blue-500"
            />
          </div>
          <div>
            <label className="block text-sm font-medium mb-2">Group</label>
            <select
              value={groupId}
              onChange={(e) => setGroupId(e.target.value)}
              className="w-full px-4 py-2 bg-gray-700 border border-gray-600 rounded-lg focus:outline-none focus:border-blue-500"
            >
              <option value="">No Group</option>
              {groups.map((group) => (
                <option key={group.id} value={group.id}>{group.name}</option>
              ))}
            </select>
          </div>
          <div>
            <label className="block text-sm font-medium mb-2">Labels</label>
            <input
              type="text"
              value={labels}
              onChange={(e) => setLabels(e.target.value)}
              placeholder="e.g., env=prod, region=us-east-1"
              className="w-full px-4 py-2 bg-gray-700 border border-gray-600 rounded-lg focus:outline-none focus:border-blue-500"
            />
            <p className="text-xs text-gray-500 mt-1">Comma-separated key=value pairs</p>
          </div>
          <div className="flex gap-3 pt-4">
            <button
              type="button"
              onClick={handleClose}
              className="flex-1 py-2 bg-gray-700 hover:bg-gray-600 rounded-lg transition-colors"
            >
              Cancel
            </button>
            <button
              type="submit"
              disabled={saving || !hostname}
              className="flex-1 py-2 bg-blue-600 hover:bg-blue-700 disabled:opacity-50 rounded-lg transition-colors"
            >
              {saving ? 'Registering...' : 'Register Agent'}
            </button>
          </div>
        </form>
      )}
    </Modal>
  );
}

// Push Config Modal
function PushConfigModal({
  isOpen,
  onClose,
  config,
  groups,
  agents,
  environments,
  onRefresh,
}: {
  isOpen: boolean;
  onClose: () => void;
  config: Config | null;
  groups: Group[];
  agents: Agent[];
  environments: Environment[];
  onRefresh: () => void;
}) {
  const [targetType, setTargetType] = useState<'agent' | 'group' | 'environment'>('group');
  const [targetId, setTargetId] = useState('');
  const [pushing, setPushing] = useState(false);
  const [result, setResult] = useState<{ success: boolean; message: string } | null>(null);

  const handlePush = async () => {
    if (!config || !targetId) return;

    setPushing(true);
    try {
      await opampApi.pushConfig(config.id, targetType, targetId);
      setResult({ success: true, message: 'Configuration pushed successfully!' });
      onRefresh();
    } catch (error) {
      setResult({ success: false, message: 'Failed to push configuration' });
    } finally {
      setPushing(false);
    }
  };

  const handleClose = () => {
    setTargetType('group');
    setTargetId('');
    setResult(null);
    onClose();
  };

  const targets = targetType === 'agent' ? agents : targetType === 'group' ? groups : environments;

  return (
    <Modal isOpen={isOpen} onClose={handleClose} title={`Push Configuration: ${config?.name || ''}`}>
      {result ? (
        <div className="space-y-4">
          <div className={clsx('flex items-center gap-2', result.success ? 'text-green-400' : 'text-red-400')}>
            {result.success ? <CheckCircle className="w-5 h-5" /> : <XCircle className="w-5 h-5" />}
            <span>{result.message}</span>
          </div>
          <button
            onClick={handleClose}
            className="w-full py-2 bg-blue-600 hover:bg-blue-700 rounded-lg transition-colors"
          >
            Done
          </button>
        </div>
      ) : (
        <div className="space-y-4">
          <div>
            <label className="block text-sm font-medium mb-2">Target Type</label>
            <div className="flex gap-2">
              {(['group', 'agent', 'environment'] as const).map((type) => (
                <button
                  key={type}
                  onClick={() => { setTargetType(type); setTargetId(''); }}
                  className={clsx(
                    'flex-1 py-2 rounded-lg transition-colors capitalize',
                    targetType === type ? 'bg-blue-600' : 'bg-gray-700 hover:bg-gray-600'
                  )}
                >
                  {type}
                </button>
              ))}
            </div>
          </div>
          <div>
            <label className="block text-sm font-medium mb-2">Select {targetType}</label>
            <select
              value={targetId}
              onChange={(e) => setTargetId(e.target.value)}
              className="w-full px-4 py-2 bg-gray-700 border border-gray-600 rounded-lg focus:outline-none focus:border-blue-500"
            >
              <option value="">Select a {targetType}...</option>
              {targets.map((t) => (
                <option key={t.id} value={t.id}>{'hostname' in t ? t.hostname : t.name}</option>
              ))}
            </select>
          </div>
          <div className="p-4 bg-gray-900 rounded-lg">
            <p className="text-sm text-gray-400 mb-2">Configuration Preview</p>
            <pre className="text-xs font-mono text-gray-300 max-h-32 overflow-auto">
              {config?.config_yaml?.slice(0, 500)}...
            </pre>
          </div>
          <div className="flex gap-3 pt-4">
            <button
              type="button"
              onClick={handleClose}
              className="flex-1 py-2 bg-gray-700 hover:bg-gray-600 rounded-lg transition-colors"
            >
              Cancel
            </button>
            <button
              onClick={handlePush}
              disabled={pushing || !targetId}
              className="flex-1 py-2 bg-green-600 hover:bg-green-700 disabled:opacity-50 rounded-lg transition-colors flex items-center justify-center gap-2"
            >
              <Upload className="w-4 h-4" />
              {pushing ? 'Pushing...' : 'Push Config'}
            </button>
          </div>
        </div>
      )}
    </Modal>
  );
}
