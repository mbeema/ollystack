// API client for OllyStack services
import { getApiUrl, getOpampUrl } from './config';

const API_URL = getApiUrl();
const OPAMP_URL = getOpampUrl();

// Types for OpAMP Control Plane
export interface Environment {
  id: string;
  name: string;
  description?: string;
  variables: Record<string, string>;
  created_at: string;
  updated_at: string;
}

export interface Group {
  id: string;
  name: string;
  description?: string;
  environment_id?: string;
  config_id?: string;
  labels: Record<string, string>;
  agent_count: number;
  created_at: string;
  updated_at: string;
}

export interface Config {
  id: string;
  name: string;
  description?: string;
  config_yaml: string;
  config_hash: string;
  version: number;
  status: 'draft' | 'active' | 'archived';
  labels: Record<string, string>;
  created_at: string;
  updated_at: string;
}

export interface Agent {
  id: string;
  hostname: string;
  ip_address?: string;
  group_id?: string;
  status: 'pending' | 'connected' | 'healthy' | 'unhealthy' | 'disconnected';
  labels: Record<string, string>;
  capabilities: string[];
  effective_config_hash?: string;
  last_seen?: string;
  last_error?: string;
  created_at: string;
  updated_at: string;
}

export interface FleetStatus {
  agents: {
    total: number;
    healthy: number;
    unhealthy: number;
    connected: number;
    disconnected: number;
    pending: number;
  };
  configs: number;
  groups: number;
  environments: number;
  timestamp: string;
}

export interface ConfigTemplate {
  id: string;
  name: string;
  description: string;
  config_yaml: string;
}

// Types for Observability Data
export interface LogEntry {
  timestamp: string;
  body: string;
  severity_text: string;
  severity_number: number;
  service_name: string;
  trace_id?: string;
  span_id?: string;
  attributes: Record<string, string>;
  resource_attributes: Record<string, string>;
}

export interface MetricDataPoint {
  timestamp: string;
  value: number;
  labels: Record<string, string>;
}

export interface ServiceMetrics {
  service_name: string;
  request_count: number;
  error_count: number;
  error_rate: number;
  avg_latency_ms: number;
  p50_latency_ms: number;
  p95_latency_ms: number;
  p99_latency_ms: number;
}

export interface ServiceTopology {
  source_service: string;
  target_service: string;
  request_count: number;
  error_count: number;
  avg_latency_ms: number;
  connection_type: string;
}

export interface Alert {
  id: string;
  name: string;
  type: string;
  severity: 'critical' | 'warning' | 'info';
  status: 'firing' | 'resolved' | 'acknowledged';
  service_name: string;
  summary: string;
  description: string;
  current_value: number;
  threshold_value: number;
  timestamp: string;
  resolved_at?: string;
}

// OpAMP Control Plane API
export const opampApi = {
  // Health
  async getHealth() {
    const res = await fetch(`${OPAMP_URL}/api/v1/health`);
    return res.json();
  },

  // Fleet Status
  async getFleetStatus(): Promise<FleetStatus> {
    const res = await fetch(`${OPAMP_URL}/api/v1/fleet/status`);
    return res.json();
  },

  // Environments
  async getEnvironments(): Promise<Environment[]> {
    const res = await fetch(`${OPAMP_URL}/api/v1/environments`);
    return res.json();
  },

  async createEnvironment(data: Omit<Environment, 'id' | 'created_at' | 'updated_at'>): Promise<Environment> {
    const res = await fetch(`${OPAMP_URL}/api/v1/environments`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(data),
    });
    return res.json();
  },

  async updateEnvironment(id: string, data: Partial<Environment>): Promise<Environment> {
    const res = await fetch(`${OPAMP_URL}/api/v1/environments/${id}`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(data),
    });
    return res.json();
  },

  async deleteEnvironment(id: string): Promise<void> {
    await fetch(`${OPAMP_URL}/api/v1/environments/${id}`, { method: 'DELETE' });
  },

  // Groups
  async getGroups(environmentId?: string): Promise<Group[]> {
    const url = environmentId
      ? `${OPAMP_URL}/api/v1/groups?environment_id=${environmentId}`
      : `${OPAMP_URL}/api/v1/groups`;
    const res = await fetch(url);
    return res.json();
  },

  async createGroup(data: Omit<Group, 'id' | 'agent_count' | 'created_at' | 'updated_at'>): Promise<Group> {
    const res = await fetch(`${OPAMP_URL}/api/v1/groups`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(data),
    });
    return res.json();
  },

  async updateGroup(id: string, data: Partial<Group>): Promise<Group> {
    const res = await fetch(`${OPAMP_URL}/api/v1/groups/${id}`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(data),
    });
    return res.json();
  },

  async deleteGroup(id: string): Promise<void> {
    await fetch(`${OPAMP_URL}/api/v1/groups/${id}`, { method: 'DELETE' });
  },

  // Configs
  async getConfigs(status?: string): Promise<Config[]> {
    const url = status
      ? `${OPAMP_URL}/api/v1/configs?status=${status}`
      : `${OPAMP_URL}/api/v1/configs`;
    const res = await fetch(url);
    return res.json();
  },

  async getConfig(id: string): Promise<Config> {
    const res = await fetch(`${OPAMP_URL}/api/v1/configs/${id}`);
    return res.json();
  },

  async createConfig(data: { name: string; description?: string; config_yaml: string; labels?: Record<string, string> }): Promise<Config> {
    const res = await fetch(`${OPAMP_URL}/api/v1/configs`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(data),
    });
    return res.json();
  },

  async updateConfig(id: string, data: Partial<Config>): Promise<Config> {
    const res = await fetch(`${OPAMP_URL}/api/v1/configs/${id}`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(data),
    });
    return res.json();
  },

  async activateConfig(id: string): Promise<Config> {
    const res = await fetch(`${OPAMP_URL}/api/v1/configs/${id}/activate`, { method: 'POST' });
    return res.json();
  },

  async deleteConfig(id: string): Promise<void> {
    await fetch(`${OPAMP_URL}/api/v1/configs/${id}`, { method: 'DELETE' });
  },

  async pushConfig(configId: string, targetType: 'agent' | 'group' | 'environment', targetId: string) {
    const res = await fetch(`${OPAMP_URL}/api/v1/configs/push`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ config_id: configId, target_type: targetType, target_id: targetId }),
    });
    return res.json();
  },

  // Agents
  async getAgents(groupId?: string, status?: string): Promise<Agent[]> {
    const params = new URLSearchParams();
    if (groupId) params.append('group_id', groupId);
    if (status) params.append('status', status);
    const url = params.toString()
      ? `${OPAMP_URL}/api/v1/agents?${params}`
      : `${OPAMP_URL}/api/v1/agents`;
    const res = await fetch(url);
    return res.json();
  },

  async getAgent(id: string): Promise<Agent> {
    const res = await fetch(`${OPAMP_URL}/api/v1/agents/${id}`);
    return res.json();
  },

  async registerAgent(data: { agent_id?: string; hostname: string; ip_address?: string; group_id?: string; labels?: Record<string, string>; capabilities?: string[] }): Promise<Agent> {
    const res = await fetch(`${OPAMP_URL}/api/v1/agents/register`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(data),
    });
    return res.json();
  },

  async updateAgent(id: string, data: Partial<Agent>): Promise<Agent> {
    const res = await fetch(`${OPAMP_URL}/api/v1/agents/${id}`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(data),
    });
    return res.json();
  },

  async deleteAgent(id: string): Promise<void> {
    await fetch(`${OPAMP_URL}/api/v1/agents/${id}`, { method: 'DELETE' });
  },

  // Templates
  async getTemplates(): Promise<{ templates: ConfigTemplate[] }> {
    const res = await fetch(`${OPAMP_URL}/api/v1/templates`);
    return res.json();
  },
};

// Observability Data API
export const observabilityApi = {
  // Logs
  async getLogs(params: {
    service_name?: string;
    severity?: string;
    trace_id?: string;
    search?: string;
    start_time?: string;
    end_time?: string;
    limit?: number;
  }): Promise<LogEntry[]> {
    const searchParams = new URLSearchParams();
    Object.entries(params).forEach(([key, value]) => {
      if (value !== undefined) searchParams.append(key, String(value));
    });
    const res = await fetch(`${API_URL}/api/v1/logs?${searchParams}`);
    return res.json();
  },

  async getLogsByTraceId(traceId: string): Promise<LogEntry[]> {
    const res = await fetch(`${API_URL}/api/v1/logs?trace_id=${traceId}`);
    return res.json();
  },

  // Metrics
  async getServiceMetrics(params: {
    service_name?: string;
    start_time?: string;
    end_time?: string;
  }): Promise<ServiceMetrics[]> {
    const searchParams = new URLSearchParams();
    Object.entries(params).forEach(([key, value]) => {
      if (value !== undefined) searchParams.append(key, String(value));
    });
    const res = await fetch(`${API_URL}/api/v1/metrics/services?${searchParams}`);
    return res.json();
  },

  async getMetricTimeSeries(params: {
    metric_name: string;
    service_name?: string;
    start_time: string;
    end_time: string;
    interval?: string;
  }): Promise<MetricDataPoint[]> {
    const searchParams = new URLSearchParams();
    Object.entries(params).forEach(([key, value]) => {
      if (value !== undefined) searchParams.append(key, String(value));
    });
    const res = await fetch(`${API_URL}/api/v1/metrics/timeseries?${searchParams}`);
    return res.json();
  },

  // Service Topology
  async getServiceTopology(params: {
    start_time?: string;
    end_time?: string;
  }): Promise<ServiceTopology[]> {
    const searchParams = new URLSearchParams();
    Object.entries(params).forEach(([key, value]) => {
      if (value !== undefined) searchParams.append(key, String(value));
    });
    const res = await fetch(`${API_URL}/api/v1/topology?${searchParams}`);
    return res.json();
  },

  // Alerts
  async getAlerts(params: {
    status?: string;
    severity?: string;
    service_name?: string;
    limit?: number;
  }): Promise<Alert[]> {
    const searchParams = new URLSearchParams();
    Object.entries(params).forEach(([key, value]) => {
      if (value !== undefined) searchParams.append(key, String(value));
    });
    const res = await fetch(`${API_URL}/api/v1/alerts?${searchParams}`);
    return res.json();
  },

  async acknowledgeAlert(id: string): Promise<Alert> {
    const res = await fetch(`${API_URL}/api/v1/alerts/${id}/acknowledge`, { method: 'POST' });
    return res.json();
  },

  async resolveAlert(id: string, resolution: string): Promise<Alert> {
    const res = await fetch(`${API_URL}/api/v1/alerts/${id}/resolve`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ resolution }),
    });
    return res.json();
  },
};
