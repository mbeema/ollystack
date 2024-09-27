import { useEffect, useRef, useState } from 'react';
import { AlertTriangle, CheckCircle, Clock, ArrowRight } from 'lucide-react';
import { clsx } from 'clsx';

interface ServiceNode {
  name: string;
  spanCount: number;
  errorCount: number;
  avgLatency: number;
  status: 'healthy' | 'degraded' | 'error';
}

interface ServiceEdge {
  source: string;
  target: string;
  callCount: number;
  avgLatency: number;
  errorRate: number;
}

interface ServiceFlowDiagramProps {
  services: string[];
  timeline?: Array<{
    timestamp: string;
    service: string;
    operation: string;
    status: string;
    duration?: number;
  }>;
  traces?: Array<{
    traceId: string;
    serviceName: string;
    operationName: string;
    duration: number;
    status: string;
  }>;
}

export default function ServiceFlowDiagram({ services, timeline, traces }: ServiceFlowDiagramProps) {
  const [nodes, setNodes] = useState<ServiceNode[]>([]);
  const [edges, setEdges] = useState<ServiceEdge[]>([]);

  useEffect(() => {
    if (!services || services.length === 0) return;

    // Build nodes from services
    const nodeMap = new Map<string, ServiceNode>();

    services.forEach(service => {
      nodeMap.set(service, {
        name: service,
        spanCount: 0,
        errorCount: 0,
        avgLatency: 0,
        status: 'healthy',
      });
    });

    // Analyze timeline to get metrics and flow
    const edgeMap = new Map<string, ServiceEdge>();
    let prevService: string | null = null;

    if (timeline && timeline.length > 0) {
      timeline.forEach(item => {
        const node = nodeMap.get(item.service);
        if (node) {
          node.spanCount++;
          if (item.status === 'error') {
            node.errorCount++;
          }
          if (item.duration) {
            node.avgLatency = (node.avgLatency * (node.spanCount - 1) + item.duration / 1000000) / node.spanCount;
          }
        }

        // Build edges from sequential service calls
        if (prevService && prevService !== item.service) {
          const edgeKey = `${prevService}->${item.service}`;
          const existing = edgeMap.get(edgeKey);
          if (existing) {
            existing.callCount++;
            if (item.status === 'error') {
              existing.errorRate = (existing.errorRate * (existing.callCount - 1) + 1) / existing.callCount;
            }
          } else {
            edgeMap.set(edgeKey, {
              source: prevService,
              target: item.service,
              callCount: 1,
              avgLatency: item.duration ? item.duration / 1000000 : 0,
              errorRate: item.status === 'error' ? 1 : 0,
            });
          }
        }
        prevService = item.service;
      });
    }

    // Also analyze traces for additional metrics
    if (traces && traces.length > 0) {
      traces.forEach(trace => {
        const node = nodeMap.get(trace.serviceName);
        if (node) {
          node.spanCount++;
          if (trace.status === 'error') {
            node.errorCount++;
          }
          node.avgLatency = (node.avgLatency * (node.spanCount - 1) + trace.duration / 1000000) / node.spanCount;
        }
      });
    }

    // Determine node status
    nodeMap.forEach(node => {
      const errorRate = node.spanCount > 0 ? node.errorCount / node.spanCount : 0;
      if (errorRate > 0.1) {
        node.status = 'error';
      } else if (errorRate > 0 || node.avgLatency > 500) {
        node.status = 'degraded';
      } else {
        node.status = 'healthy';
      }
    });

    setNodes(Array.from(nodeMap.values()));
    setEdges(Array.from(edgeMap.values()));
  }, [services, timeline, traces]);

  const getStatusColor = (status: string) => {
    switch (status) {
      case 'healthy': return 'border-green-500 bg-green-900/20';
      case 'degraded': return 'border-yellow-500 bg-yellow-900/20';
      case 'error': return 'border-red-500 bg-red-900/20';
      default: return 'border-gray-500 bg-gray-800';
    }
  };

  const getStatusIcon = (status: string) => {
    switch (status) {
      case 'healthy': return <CheckCircle className="w-4 h-4 text-green-400" />;
      case 'degraded': return <AlertTriangle className="w-4 h-4 text-yellow-400" />;
      case 'error': return <AlertTriangle className="w-4 h-4 text-red-400" />;
      default: return null;
    }
  };

  if (nodes.length === 0) {
    return (
      <div className="flex items-center justify-center h-48 text-gray-400">
        <p className="text-sm">No service flow data available</p>
      </div>
    );
  }

  // Simple horizontal layout
  const nodeWidth = 160;
  const nodeHeight = 80;
  const nodeSpacing = 40;
  const totalWidth = nodes.length * (nodeWidth + nodeSpacing);

  return (
    <div className="overflow-x-auto">
      <div className="min-w-max p-4">
        {/* Service Flow Header */}
        <div className="flex items-center gap-2 mb-4 text-sm text-gray-400">
          <span>Request Flow</span>
          <ArrowRight className="w-4 h-4" />
        </div>

        {/* Flow Diagram */}
        <div className="relative flex items-center gap-2">
          {nodes.map((node, idx) => (
            <div key={node.name} className="flex items-center">
              {/* Node */}
              <div
                className={clsx(
                  'relative p-4 rounded-lg border-2 transition-all hover:scale-105',
                  getStatusColor(node.status)
                )}
                style={{ width: nodeWidth, minHeight: nodeHeight }}
              >
                {/* Status Icon */}
                <div className="absolute -top-2 -right-2">
                  {getStatusIcon(node.status)}
                </div>

                {/* Service Name */}
                <div className="font-medium text-sm text-white truncate mb-2">
                  {node.name}
                </div>

                {/* Metrics */}
                <div className="space-y-1 text-xs text-gray-400">
                  <div className="flex items-center justify-between">
                    <span>Spans:</span>
                    <span className="text-white">{node.spanCount}</span>
                  </div>
                  <div className="flex items-center justify-between">
                    <span>Latency:</span>
                    <span className="text-white">{node.avgLatency.toFixed(0)}ms</span>
                  </div>
                  {node.errorCount > 0 && (
                    <div className="flex items-center justify-between text-red-400">
                      <span>Errors:</span>
                      <span>{node.errorCount}</span>
                    </div>
                  )}
                </div>
              </div>

              {/* Arrow to next node */}
              {idx < nodes.length - 1 && (
                <div className="flex items-center px-2">
                  <div className="w-8 h-0.5 bg-gray-600"></div>
                  <ArrowRight className="w-4 h-4 text-gray-500 -ml-1" />
                </div>
              )}
            </div>
          ))}
        </div>

        {/* Edge Details */}
        {edges.length > 0 && (
          <div className="mt-6 pt-4 border-t border-gray-700">
            <div className="text-sm text-gray-400 mb-3">Service Calls</div>
            <div className="grid grid-cols-2 gap-2">
              {edges.map((edge, idx) => (
                <div
                  key={idx}
                  className="flex items-center gap-2 p-2 bg-gray-750 rounded text-xs"
                >
                  <span className="text-blue-400">{edge.source}</span>
                  <ArrowRight className="w-3 h-3 text-gray-500" />
                  <span className="text-green-400">{edge.target}</span>
                  <span className="ml-auto text-gray-400">
                    {edge.callCount} calls
                  </span>
                </div>
              ))}
            </div>
          </div>
        )}

        {/* Legend */}
        <div className="mt-4 pt-4 border-t border-gray-700 flex items-center gap-6 text-xs text-gray-400">
          <div className="flex items-center gap-2">
            <div className="w-3 h-3 rounded border-2 border-green-500 bg-green-900/20"></div>
            <span>Healthy</span>
          </div>
          <div className="flex items-center gap-2">
            <div className="w-3 h-3 rounded border-2 border-yellow-500 bg-yellow-900/20"></div>
            <span>Degraded</span>
          </div>
          <div className="flex items-center gap-2">
            <div className="w-3 h-3 rounded border-2 border-red-500 bg-red-900/20"></div>
            <span>Error</span>
          </div>
        </div>
      </div>
    </div>
  );
}
