import { useEffect, useRef, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { useQuery } from '@tanstack/react-query';
import * as d3 from 'd3';
import { Loader2, GitBranch, AlertTriangle, Activity, Clock, ArrowRight, RefreshCw } from 'lucide-react';
import { getApiUrl } from '../lib/config';

interface ServiceNode {
  name: string;
  type: string;
  avgLatency: number;
  p50Latency: number;
  p95Latency: number;
  p99Latency: number;
  errorRate: number;
  requestRate: number;
  totalSpans: number;
}

interface ServiceEdge {
  source: string;
  target: string;
  requestCount: number;
  errorRate: number;
  avgLatency: number;
  p95Latency: number;
}

interface TopologyData {
  services: Record<string, ServiceNode>;
  edges: Record<string, ServiceEdge>;
}

const API_URL = getApiUrl();

async function fetchTopology(): Promise<TopologyData> {
  const response = await fetch(`${API_URL}/api/v1/topology/graph`);
  if (!response.ok) {
    throw new Error('Failed to fetch topology');
  }
  return response.json();
}

// Format latency for display
function formatLatency(ms: number): string {
  if (ms < 1) return '<1ms';
  if (ms < 1000) return `${Math.round(ms)}ms`;
  return `${(ms / 1000).toFixed(1)}s`;
}

// Format request rate
function formatRate(rate: number): string {
  if (rate < 1) return `${(rate * 60).toFixed(1)}/min`;
  return `${rate.toFixed(1)}/s`;
}

// Get node color based on error rate and type
function getNodeColor(errorRate: number, type: string): string {
  if (errorRate > 5) return '#dc2626'; // Red for high errors
  if (errorRate > 1) return '#f59e0b'; // Amber for moderate errors

  switch (type) {
    case 'database': return '#8b5cf6'; // Purple
    case 'queue': return '#06b6d4'; // Cyan
    case 'gateway': return '#10b981'; // Emerald
    case 'client': return '#6b7280'; // Gray
    default: return '#3b82f6'; // Blue
  }
}

// Get edge color based on error rate
function getEdgeColor(errorRate: number): string {
  if (errorRate > 5) return '#dc2626';
  if (errorRate > 1) return '#f59e0b';
  return '#4b5563';
}

// Get node icon based on type
function getNodeIcon(type: string): string {
  switch (type) {
    case 'database': return '\u{1F5C4}'; // Filing cabinet
    case 'queue': return '\u{1F4E8}'; // Envelope
    case 'gateway': return '\u{1F6AA}'; // Door
    case 'client': return '\u{1F4BB}'; // Laptop
    default: return '\u{2699}'; // Gear
  }
}

export default function ServiceMapPage() {
  const navigate = useNavigate();
  const svgRef = useRef<SVGSVGElement>(null);
  const [selectedService, setSelectedService] = useState<string | null>(null);
  const [selectedEdge, setSelectedEdge] = useState<ServiceEdge | null>(null);

  const { data, isLoading, error, refetch, dataUpdatedAt } = useQuery({
    queryKey: ['topology'],
    queryFn: fetchTopology,
    refetchInterval: 30000,
  });

  const hasData = data && Object.keys(data.services || {}).length > 0;
  const hasEdges = data && Object.keys(data.edges || {}).length > 0;

  const selectedServiceData = selectedService && data?.services[selectedService];

  // D3 visualization
  useEffect(() => {
    if (!svgRef.current || !data) return;

    const svg = d3.select(svgRef.current);
    svg.selectAll('*').remove();

    const width = svgRef.current.clientWidth;
    const height = svgRef.current.clientHeight;

    // Convert data to D3 format
    const nodes = Object.entries(data.services || {}).map(([id, node]) => ({
      id,
      ...node,
    }));

    const links = Object.values(data.edges || {}).map((edge) => ({
      ...edge,
    }));

    if (nodes.length === 0) return;

    // Create arrow marker for edges
    svg.append('defs').append('marker')
      .attr('id', 'arrowhead')
      .attr('viewBox', '-0 -5 10 10')
      .attr('refX', 40)
      .attr('refY', 0)
      .attr('orient', 'auto')
      .attr('markerWidth', 8)
      .attr('markerHeight', 8)
      .append('path')
      .attr('d', 'M 0,-5 L 10,0 L 0,5')
      .attr('fill', '#6b7280');

    // Create arrow marker for error edges
    svg.append('defs').append('marker')
      .attr('id', 'arrowhead-error')
      .attr('viewBox', '-0 -5 10 10')
      .attr('refX', 40)
      .attr('refY', 0)
      .attr('orient', 'auto')
      .attr('markerWidth', 8)
      .attr('markerHeight', 8)
      .append('path')
      .attr('d', 'M 0,-5 L 10,0 L 0,5')
      .attr('fill', '#dc2626');

    // Create force simulation
    const simulation = d3
      .forceSimulation(nodes as any)
      .force('link', d3.forceLink(links).id((d: any) => d.id).distance(200))
      .force('charge', d3.forceManyBody().strength(-800))
      .force('center', d3.forceCenter(width / 2, height / 2))
      .force('collision', d3.forceCollide().radius(60));

    // Add zoom
    const g = svg.append('g');
    svg.call(
      d3.zoom<SVGSVGElement, unknown>()
        .scaleExtent([0.1, 4])
        .on('zoom', (event) => {
          g.attr('transform', event.transform);
        }) as any
    );

    // Draw links (edges)
    const link = g
      .append('g')
      .attr('class', 'links')
      .selectAll('g')
      .data(links)
      .join('g')
      .style('cursor', 'pointer')
      .on('click', (event, d) => {
        event.stopPropagation();
        setSelectedEdge(d as ServiceEdge);
        setSelectedService(null);
      });

    // Edge lines
    link.append('line')
      .attr('stroke', (d) => getEdgeColor(d.errorRate))
      .attr('stroke-width', (d) => Math.max(2, Math.min(8, Math.log(d.requestCount / 100) + 2)))
      .attr('stroke-opacity', 0.7)
      .attr('marker-end', (d) => d.errorRate > 5 ? 'url(#arrowhead-error)' : 'url(#arrowhead)');

    // Edge labels (latency)
    const edgeLabels = link.append('text')
      .attr('class', 'edge-label')
      .attr('text-anchor', 'middle')
      .attr('dy', -8)
      .attr('fill', '#9ca3af')
      .attr('font-size', '10px')
      .text((d) => formatLatency(d.avgLatency));

    // Draw nodes
    const node = g
      .append('g')
      .attr('class', 'nodes')
      .selectAll('g')
      .data(nodes)
      .join('g')
      .style('cursor', 'pointer')
      .on('click', (event, d) => {
        event.stopPropagation();
        setSelectedService(d.id);
        setSelectedEdge(null);
      })
      .call(
        d3.drag<SVGGElement, any>()
          .on('start', (event) => {
            if (!event.active) simulation.alphaTarget(0.3).restart();
            event.subject.fx = event.subject.x;
            event.subject.fy = event.subject.y;
          })
          .on('drag', (event) => {
            event.subject.fx = event.x;
            event.subject.fy = event.y;
          })
          .on('end', (event) => {
            if (!event.active) simulation.alphaTarget(0);
            event.subject.fx = null;
            event.subject.fy = null;
          }) as any
      );

    // Node circles
    node
      .append('circle')
      .attr('r', 32)
      .attr('fill', (d) => getNodeColor(d.errorRate, d.type))
      .attr('stroke', '#fff')
      .attr('stroke-width', 3)
      .attr('filter', 'drop-shadow(0 4px 6px rgba(0, 0, 0, 0.3))');

    // Error indicator ring
    node
      .filter((d) => d.errorRate > 1)
      .append('circle')
      .attr('r', 38)
      .attr('fill', 'none')
      .attr('stroke', d => d.errorRate > 5 ? '#dc2626' : '#f59e0b')
      .attr('stroke-width', 2)
      .attr('stroke-dasharray', '4,4')
      .attr('class', 'pulse-ring');

    // Node icons
    node
      .append('text')
      .text((d) => getNodeIcon(d.type))
      .attr('text-anchor', 'middle')
      .attr('dy', 6)
      .attr('font-size', '20px');

    // Node labels (service name)
    node
      .append('text')
      .text((d) => d.name)
      .attr('text-anchor', 'middle')
      .attr('dy', 52)
      .attr('fill', '#e5e7eb')
      .attr('font-size', '12px')
      .attr('font-weight', '500');

    // Request rate badge
    node
      .append('text')
      .text((d) => formatRate(d.requestRate))
      .attr('text-anchor', 'middle')
      .attr('dy', 68)
      .attr('fill', '#9ca3af')
      .attr('font-size', '10px');

    // Update positions on tick
    simulation.on('tick', () => {
      link.select('line')
        .attr('x1', (d: any) => d.source.x)
        .attr('y1', (d: any) => d.source.y)
        .attr('x2', (d: any) => d.target.x)
        .attr('y2', (d: any) => d.target.y);

      edgeLabels
        .attr('x', (d: any) => (d.source.x + d.target.x) / 2)
        .attr('y', (d: any) => (d.source.y + d.target.y) / 2);

      node.attr('transform', (d: any) => `translate(${d.x},${d.y})`);
    });

    // Clear selection when clicking on background
    svg.on('click', () => {
      setSelectedService(null);
      setSelectedEdge(null);
    });

    return () => {
      simulation.stop();
    };
  }, [data]);

  return (
    <div className="h-[calc(100vh-8rem)]">
      <div className="flex items-center justify-between mb-6">
        <div>
          <h1 className="text-2xl font-bold">Service Map</h1>
          <p className="text-sm text-gray-400 mt-1">
            Real-time service topology based on trace data
          </p>
        </div>
        <div className="flex items-center space-x-6">
          {/* Legend */}
          <div className="flex items-center space-x-4 text-sm">
            <div className="flex items-center space-x-2">
              <span className="w-3 h-3 bg-blue-500 rounded-full"></span>
              <span className="text-gray-400">Service</span>
            </div>
            <div className="flex items-center space-x-2">
              <span className="w-3 h-3 bg-emerald-500 rounded-full"></span>
              <span className="text-gray-400">Gateway</span>
            </div>
            <div className="flex items-center space-x-2">
              <span className="w-3 h-3 bg-purple-500 rounded-full"></span>
              <span className="text-gray-400">Database</span>
            </div>
            <div className="flex items-center space-x-2">
              <span className="w-3 h-3 bg-red-600 rounded-full"></span>
              <span className="text-gray-400">Error</span>
            </div>
          </div>

          {/* Refresh button */}
          <button
            onClick={() => refetch()}
            className="flex items-center space-x-2 px-3 py-1.5 bg-gray-700 hover:bg-gray-600 rounded-lg text-sm transition-colors"
          >
            <RefreshCw className="w-4 h-4" />
            <span>Refresh</span>
          </button>
        </div>
      </div>

      <div className="bg-gray-800 rounded-lg h-[calc(100%-4rem)] relative">
        {isLoading ? (
          <div className="flex items-center justify-center h-full">
            <Loader2 className="w-8 h-8 animate-spin text-blue-500" />
          </div>
        ) : error ? (
          <div className="flex flex-col items-center justify-center h-full text-center">
            <AlertTriangle className="w-16 h-16 mb-4 text-red-400" />
            <div className="text-red-400 text-lg mb-2">Error loading topology</div>
            <div className="text-gray-500">Failed to fetch service map data</div>
          </div>
        ) : !hasData ? (
          <div className="flex flex-col items-center justify-center h-full text-center">
            <GitBranch className="w-16 h-16 mx-auto mb-4 text-gray-600" />
            <div className="text-gray-400 text-lg mb-2">No services discovered yet</div>
            <div className="text-gray-500 text-sm">Service topology will appear here once traces are collected</div>
          </div>
        ) : !hasEdges ? (
          <div className="flex flex-col items-center justify-center h-full text-center">
            <GitBranch className="w-16 h-16 mx-auto mb-4 text-gray-600" />
            <div className="text-gray-400 text-lg mb-2">Services found but no connections detected</div>
            <div className="text-gray-500 text-sm">Connections will appear once cross-service traces are collected</div>
          </div>
        ) : null}

        <svg ref={svgRef} className={`w-full h-full ${!hasData || !hasEdges ? 'hidden' : ''}`} />

        {/* Service Details Panel */}
        {selectedServiceData && (
          <div className="absolute top-4 right-4 w-80 bg-gray-900 rounded-lg border border-gray-700 shadow-xl">
            <div className="p-4 border-b border-gray-700">
              <div className="flex items-center justify-between">
                <div className="flex items-center space-x-3">
                  <div
                    className="w-10 h-10 rounded-full flex items-center justify-center text-xl"
                    style={{ backgroundColor: getNodeColor(selectedServiceData.errorRate, selectedServiceData.type) }}
                  >
                    {getNodeIcon(selectedServiceData.type)}
                  </div>
                  <div>
                    <h3 className="font-semibold text-lg">{selectedService}</h3>
                    <span className="text-sm text-gray-400 capitalize">{selectedServiceData.type}</span>
                  </div>
                </div>
                <button
                  onClick={() => setSelectedService(null)}
                  className="text-gray-400 hover:text-white p-1"
                >
                  &times;
                </button>
              </div>
            </div>

            <div className="p-4 space-y-4">
              {/* Health indicator */}
              {selectedServiceData.errorRate > 1 && (
                <div className={`flex items-center space-x-2 p-2 rounded ${
                  selectedServiceData.errorRate > 5 ? 'bg-red-900/30 text-red-400' : 'bg-amber-900/30 text-amber-400'
                }`}>
                  <AlertTriangle className="w-4 h-4" />
                  <span className="text-sm">
                    {selectedServiceData.errorRate > 5 ? 'High error rate detected' : 'Elevated error rate'}
                  </span>
                </div>
              )}

              {/* Metrics */}
              <div className="grid grid-cols-2 gap-3">
                <div className="bg-gray-800 rounded-lg p-3">
                  <div className="flex items-center space-x-2 text-gray-400 text-xs mb-1">
                    <Activity className="w-3 h-3" />
                    <span>Request Rate</span>
                  </div>
                  <div className="text-lg font-semibold">{formatRate(selectedServiceData.requestRate)}</div>
                </div>

                <div className="bg-gray-800 rounded-lg p-3">
                  <div className="flex items-center space-x-2 text-gray-400 text-xs mb-1">
                    <AlertTriangle className="w-3 h-3" />
                    <span>Error Rate</span>
                  </div>
                  <div className={`text-lg font-semibold ${
                    selectedServiceData.errorRate > 5 ? 'text-red-400' :
                    selectedServiceData.errorRate > 1 ? 'text-amber-400' : 'text-green-400'
                  }`}>
                    {selectedServiceData.errorRate.toFixed(2)}%
                  </div>
                </div>
              </div>

              {/* Latency breakdown */}
              <div className="space-y-2">
                <div className="flex items-center space-x-2 text-gray-400 text-xs">
                  <Clock className="w-3 h-3" />
                  <span>Latency</span>
                </div>
                <div className="grid grid-cols-4 gap-2 text-center">
                  <div className="bg-gray-800 rounded p-2">
                    <div className="text-xs text-gray-500">Avg</div>
                    <div className="font-medium">{formatLatency(selectedServiceData.avgLatency)}</div>
                  </div>
                  <div className="bg-gray-800 rounded p-2">
                    <div className="text-xs text-gray-500">P50</div>
                    <div className="font-medium">{formatLatency(selectedServiceData.p50Latency)}</div>
                  </div>
                  <div className="bg-gray-800 rounded p-2">
                    <div className="text-xs text-gray-500">P95</div>
                    <div className="font-medium">{formatLatency(selectedServiceData.p95Latency)}</div>
                  </div>
                  <div className="bg-gray-800 rounded p-2">
                    <div className="text-xs text-gray-500">P99</div>
                    <div className="font-medium">{formatLatency(selectedServiceData.p99Latency)}</div>
                  </div>
                </div>
              </div>

              {/* Actions */}
              <div className="pt-2 border-t border-gray-700 space-y-2">
                <button
                  onClick={() => navigate(`/traces?service=${encodeURIComponent(selectedService)}`)}
                  className="w-full px-4 py-2 bg-blue-600 hover:bg-blue-700 rounded-lg text-sm font-medium transition-colors"
                >
                  View Traces
                </button>
                <button
                  onClick={() => navigate(`/logs?service=${encodeURIComponent(selectedService)}`)}
                  className="w-full px-4 py-2 bg-gray-700 hover:bg-gray-600 rounded-lg text-sm font-medium transition-colors"
                >
                  View Logs
                </button>
              </div>
            </div>
          </div>
        )}

        {/* Edge Details Panel */}
        {selectedEdge && (
          <div className="absolute top-4 right-4 w-80 bg-gray-900 rounded-lg border border-gray-700 shadow-xl">
            <div className="p-4 border-b border-gray-700">
              <div className="flex items-center justify-between">
                <div className="flex items-center space-x-2">
                  <span className="font-medium">{selectedEdge.source}</span>
                  <ArrowRight className="w-4 h-4 text-gray-400" />
                  <span className="font-medium">{selectedEdge.target}</span>
                </div>
                <button
                  onClick={() => setSelectedEdge(null)}
                  className="text-gray-400 hover:text-white p-1"
                >
                  &times;
                </button>
              </div>
            </div>

            <div className="p-4 space-y-4">
              {selectedEdge.errorRate > 1 && (
                <div className={`flex items-center space-x-2 p-2 rounded ${
                  selectedEdge.errorRate > 5 ? 'bg-red-900/30 text-red-400' : 'bg-amber-900/30 text-amber-400'
                }`}>
                  <AlertTriangle className="w-4 h-4" />
                  <span className="text-sm">
                    {selectedEdge.errorRate.toFixed(1)}% of requests failing
                  </span>
                </div>
              )}

              <div className="space-y-3">
                <div className="flex justify-between text-sm">
                  <span className="text-gray-400">Request Count</span>
                  <span className="font-medium">{selectedEdge.requestCount.toLocaleString()}</span>
                </div>
                <div className="flex justify-between text-sm">
                  <span className="text-gray-400">Error Rate</span>
                  <span className={`font-medium ${
                    selectedEdge.errorRate > 5 ? 'text-red-400' :
                    selectedEdge.errorRate > 1 ? 'text-amber-400' : 'text-green-400'
                  }`}>
                    {selectedEdge.errorRate.toFixed(2)}%
                  </span>
                </div>
                <div className="flex justify-between text-sm">
                  <span className="text-gray-400">Avg Latency</span>
                  <span className="font-medium">{formatLatency(selectedEdge.avgLatency)}</span>
                </div>
                <div className="flex justify-between text-sm">
                  <span className="text-gray-400">P95 Latency</span>
                  <span className="font-medium">{formatLatency(selectedEdge.p95Latency)}</span>
                </div>
              </div>

              <div className="pt-2 border-t border-gray-700 space-y-2">
                <button
                  onClick={() => navigate(`/traces?service=${encodeURIComponent(selectedEdge.source)}`)}
                  className="w-full px-4 py-2 bg-blue-600 hover:bg-blue-700 rounded-lg text-sm font-medium transition-colors"
                >
                  View Traces ({selectedEdge.source})
                </button>
                <button
                  onClick={() => navigate(`/traces?service=${encodeURIComponent(selectedEdge.target)}`)}
                  className="w-full px-4 py-2 bg-gray-700 hover:bg-gray-600 rounded-lg text-sm font-medium transition-colors"
                >
                  View Traces ({selectedEdge.target})
                </button>
              </div>
            </div>
          </div>
        )}

        {/* Last updated indicator */}
        {dataUpdatedAt && (
          <div className="absolute bottom-4 left-4 text-xs text-gray-500">
            Last updated: {new Date(dataUpdatedAt).toLocaleTimeString()}
          </div>
        )}
      </div>

      <style>{`
        .pulse-ring {
          animation: pulse 2s ease-in-out infinite;
        }
        @keyframes pulse {
          0%, 100% { opacity: 1; }
          50% { opacity: 0.3; }
        }
      `}</style>
    </div>
  );
}
