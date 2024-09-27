import { useEffect, useRef, useState, useMemo, useCallback } from 'react';
import { ZoomIn, ZoomOut, Maximize2, AlertCircle, Activity } from 'lucide-react';
import { useTraceStore } from '../../stores/traceStore';
import { formatDuration } from '../../types/trace';
import type { Span } from '../../types/trace';
import { clsx } from 'clsx';

interface ServiceNode {
  id: string;
  name: string;
  color: string;
  spanCount: number;
  errorCount: number;
  totalDuration: number;
  avgDuration: number;
  x: number;
  y: number;
  vx: number;
  vy: number;
  radius: number;
}

interface ServiceEdge {
  source: string;
  target: string;
  requestCount: number;
  errorCount: number;
  avgLatency: number;
  totalLatency: number;
}

interface ServiceMapProps {
  width: number;
  height: number;
}

export function ServiceMap({ width, height }: ServiceMapProps) {
  const svgRef = useRef<SVGSVGElement>(null);
  const [zoom, setZoom] = useState(1);
  const [pan, setPan] = useState({ x: 0, y: 0 });
  const [isDragging, setIsDragging] = useState(false);
  const [dragStart, setDragStart] = useState({ x: 0, y: 0 });
  const [selectedNode, setSelectedNode] = useState<string | null>(null);
  const [hoveredNode, setHoveredNode] = useState<string | null>(null);
  const [hoveredEdge, setHoveredEdge] = useState<string | null>(null);
  const animationRef = useRef<number | null>(null);

  const { trace, selectSpan } = useTraceStore();

  // Build service graph from trace
  const { nodes, edges } = useMemo(() => {
    if (!trace) return { nodes: [], edges: [] };

    const serviceMap = new Map<string, ServiceNode>();
    const edgeMap = new Map<string, ServiceEdge>();

    // Build service nodes
    trace.spans.forEach(span => {
      if (!serviceMap.has(span.serviceName)) {
        serviceMap.set(span.serviceName, {
          id: span.serviceName,
          name: span.serviceName,
          color: span.serviceColor,
          spanCount: 0,
          errorCount: 0,
          totalDuration: 0,
          avgDuration: 0,
          x: Math.random() * (width - 100) + 50,
          y: Math.random() * (height - 100) + 50,
          vx: 0,
          vy: 0,
          radius: 30,
        });
      }

      const node = serviceMap.get(span.serviceName)!;
      node.spanCount++;
      node.totalDuration += span.duration;
      if (span.status === 'ERROR') node.errorCount++;
    });

    // Calculate average durations and radii
    serviceMap.forEach(node => {
      node.avgDuration = node.totalDuration / node.spanCount;
      // Scale radius based on span count
      node.radius = Math.max(25, Math.min(50, 20 + node.spanCount * 5));
    });

    // Build edges from parent-child relationships
    trace.spans.forEach(span => {
      if (span.parentSpanId) {
        const parentSpan = trace.spans.find(s => s.spanId === span.parentSpanId);
        if (parentSpan && parentSpan.serviceName !== span.serviceName) {
          const edgeId = `${parentSpan.serviceName}->${span.serviceName}`;

          if (!edgeMap.has(edgeId)) {
            edgeMap.set(edgeId, {
              source: parentSpan.serviceName,
              target: span.serviceName,
              requestCount: 0,
              errorCount: 0,
              avgLatency: 0,
              totalLatency: 0,
            });
          }

          const edge = edgeMap.get(edgeId)!;
          edge.requestCount++;
          edge.totalLatency += span.duration;
          if (span.status === 'ERROR') edge.errorCount++;
        }
      }
    });

    // Calculate average latencies
    edgeMap.forEach(edge => {
      edge.avgLatency = edge.totalLatency / edge.requestCount;
    });

    return {
      nodes: Array.from(serviceMap.values()),
      edges: Array.from(edgeMap.values()),
    };
  }, [trace, width, height]);

  // Simple force simulation
  const [simulatedNodes, setSimulatedNodes] = useState<ServiceNode[]>([]);

  useEffect(() => {
    if (nodes.length === 0) return;

    // Initialize positions in a circle
    const centerX = width / 2;
    const centerY = height / 2;
    const radius = Math.min(width, height) / 3;

    const initialNodes = nodes.map((node, i) => ({
      ...node,
      x: centerX + radius * Math.cos((2 * Math.PI * i) / nodes.length),
      y: centerY + radius * Math.sin((2 * Math.PI * i) / nodes.length),
      vx: 0,
      vy: 0,
    }));

    setSimulatedNodes(initialNodes);

    // Run force simulation
    let iteration = 0;
    const maxIterations = 100;
    const alpha = 0.3;
    const alphaDecay = 0.02;

    const simulate = () => {
      if (iteration >= maxIterations) return;

      setSimulatedNodes(prev => {
        const newNodes = prev.map(node => ({ ...node }));
        const currentAlpha = alpha * Math.pow(1 - alphaDecay, iteration);

        // Repulsion between all nodes
        for (let i = 0; i < newNodes.length; i++) {
          for (let j = i + 1; j < newNodes.length; j++) {
            const dx = newNodes[j].x - newNodes[i].x;
            const dy = newNodes[j].y - newNodes[i].y;
            const dist = Math.sqrt(dx * dx + dy * dy) || 1;
            const force = (150 * 150) / dist;
            const fx = (dx / dist) * force * currentAlpha;
            const fy = (dy / dist) * force * currentAlpha;
            newNodes[i].vx -= fx;
            newNodes[i].vy -= fy;
            newNodes[j].vx += fx;
            newNodes[j].vy += fy;
          }
        }

        // Attraction along edges
        edges.forEach(edge => {
          const source = newNodes.find(n => n.id === edge.source);
          const target = newNodes.find(n => n.id === edge.target);
          if (source && target) {
            const dx = target.x - source.x;
            const dy = target.y - source.y;
            const dist = Math.sqrt(dx * dx + dy * dy) || 1;
            const targetDist = 150;
            const force = (dist - targetDist) * 0.1 * currentAlpha;
            const fx = (dx / dist) * force;
            const fy = (dy / dist) * force;
            source.vx += fx;
            source.vy += fy;
            target.vx -= fx;
            target.vy -= fy;
          }
        });

        // Center gravity
        newNodes.forEach(node => {
          node.vx += (centerX - node.x) * 0.01 * currentAlpha;
          node.vy += (centerY - node.y) * 0.01 * currentAlpha;
        });

        // Apply velocities and damping
        newNodes.forEach(node => {
          node.x += node.vx;
          node.y += node.vy;
          node.vx *= 0.9;
          node.vy *= 0.9;

          // Keep nodes in bounds
          node.x = Math.max(node.radius, Math.min(width - node.radius, node.x));
          node.y = Math.max(node.radius, Math.min(height - node.radius, node.y));
        });

        return newNodes;
      });

      iteration++;
      animationRef.current = requestAnimationFrame(simulate);
    };

    animationRef.current = requestAnimationFrame(simulate);

    return () => {
      if (animationRef.current) {
        cancelAnimationFrame(animationRef.current);
      }
    };
  }, [nodes, edges, width, height]);

  // Pan and zoom handlers
  const handleMouseDown = useCallback((e: React.MouseEvent) => {
    if (e.target === svgRef.current) {
      setIsDragging(true);
      setDragStart({ x: e.clientX - pan.x, y: e.clientY - pan.y });
    }
  }, [pan]);

  const handleMouseMove = useCallback((e: React.MouseEvent) => {
    if (isDragging) {
      setPan({
        x: e.clientX - dragStart.x,
        y: e.clientY - dragStart.y,
      });
    }
  }, [isDragging, dragStart]);

  const handleMouseUp = useCallback(() => {
    setIsDragging(false);
  }, []);

  const handleWheel = useCallback((e: React.WheelEvent) => {
    e.preventDefault();
    const delta = e.deltaY > 0 ? 0.9 : 1.1;
    setZoom(prev => Math.max(0.5, Math.min(3, prev * delta)));
  }, []);

  const resetView = useCallback(() => {
    setZoom(1);
    setPan({ x: 0, y: 0 });
  }, []);

  // Get edge path
  const getEdgePath = (source: ServiceNode, target: ServiceNode) => {
    const dx = target.x - source.x;
    const dy = target.y - source.y;
    const dist = Math.sqrt(dx * dx + dy * dy);

    // Calculate points on node circumference
    const sourceX = source.x + (dx / dist) * source.radius;
    const sourceY = source.y + (dy / dist) * source.radius;
    const targetX = target.x - (dx / dist) * target.radius;
    const targetY = target.y - (dy / dist) * target.radius;

    // Curved path
    const midX = (sourceX + targetX) / 2;
    const midY = (sourceY + targetY) / 2;
    const perpX = -(targetY - sourceY) * 0.2;
    const perpY = (targetX - sourceX) * 0.2;

    return `M ${sourceX} ${sourceY} Q ${midX + perpX} ${midY + perpY} ${targetX} ${targetY}`;
  };

  // Get arrow marker position
  const getArrowPosition = (source: ServiceNode, target: ServiceNode) => {
    const dx = target.x - source.x;
    const dy = target.y - source.y;
    const dist = Math.sqrt(dx * dx + dy * dy);

    const targetX = target.x - (dx / dist) * target.radius;
    const targetY = target.y - (dy / dist) * target.radius;
    const angle = Math.atan2(dy, dx) * (180 / Math.PI);

    return { x: targetX, y: targetY, angle };
  };

  if (!trace) {
    return (
      <div className="h-full flex items-center justify-center text-gray-500">
        <Activity className="w-8 h-8 mr-2 opacity-50" />
        No trace data
      </div>
    );
  }

  return (
    <div className="h-full flex flex-col bg-gray-900">
      {/* Controls */}
      <div className="flex-shrink-0 flex items-center justify-between px-3 py-2 border-b border-gray-700">
        <div className="text-sm text-gray-400">
          {simulatedNodes.length} services, {edges.length} connections
        </div>
        <div className="flex items-center gap-1">
          <button
            onClick={() => setZoom(prev => Math.min(3, prev * 1.2))}
            className="p-2 rounded-lg hover:bg-gray-700 text-gray-400 hover:text-white"
            title="Zoom in"
          >
            <ZoomIn className="w-4 h-4" />
          </button>
          <button
            onClick={() => setZoom(prev => Math.max(0.5, prev * 0.8))}
            className="p-2 rounded-lg hover:bg-gray-700 text-gray-400 hover:text-white"
            title="Zoom out"
          >
            <ZoomOut className="w-4 h-4" />
          </button>
          <button
            onClick={resetView}
            className="p-2 rounded-lg hover:bg-gray-700 text-gray-400 hover:text-white"
            title="Reset view"
          >
            <Maximize2 className="w-4 h-4" />
          </button>
        </div>
      </div>

      {/* Graph */}
      <div className="flex-1 overflow-hidden">
        <svg
          ref={svgRef}
          width={width}
          height={height - 50}
          className="cursor-grab active:cursor-grabbing"
          onMouseDown={handleMouseDown}
          onMouseMove={handleMouseMove}
          onMouseUp={handleMouseUp}
          onMouseLeave={handleMouseUp}
          onWheel={handleWheel}
        >
          <defs>
            {/* Arrow marker */}
            <marker
              id="arrowhead"
              markerWidth="10"
              markerHeight="7"
              refX="9"
              refY="3.5"
              orient="auto"
            >
              <polygon
                points="0 0, 10 3.5, 0 7"
                fill="#6b7280"
              />
            </marker>
            <marker
              id="arrowhead-error"
              markerWidth="10"
              markerHeight="7"
              refX="9"
              refY="3.5"
              orient="auto"
            >
              <polygon
                points="0 0, 10 3.5, 0 7"
                fill="#ef4444"
              />
            </marker>
            <marker
              id="arrowhead-selected"
              markerWidth="10"
              markerHeight="7"
              refX="9"
              refY="3.5"
              orient="auto"
            >
              <polygon
                points="0 0, 10 3.5, 0 7"
                fill="#3b82f6"
              />
            </marker>
          </defs>

          <g transform={`translate(${pan.x}, ${pan.y}) scale(${zoom})`}>
            {/* Edges */}
            {edges.map(edge => {
              const source = simulatedNodes.find(n => n.id === edge.source);
              const target = simulatedNodes.find(n => n.id === edge.target);
              if (!source || !target) return null;

              const edgeId = `${edge.source}->${edge.target}`;
              const isHovered = hoveredEdge === edgeId;
              const isConnected = hoveredNode === edge.source || hoveredNode === edge.target ||
                                  selectedNode === edge.source || selectedNode === edge.target;
              const hasError = edge.errorCount > 0;

              return (
                <g key={edgeId}>
                  <path
                    d={getEdgePath(source, target)}
                    fill="none"
                    stroke={hasError ? '#ef4444' : isHovered || isConnected ? '#3b82f6' : '#4b5563'}
                    strokeWidth={isHovered ? 3 : 2}
                    strokeDasharray={hasError ? '5,5' : undefined}
                    markerEnd={`url(#arrowhead${hasError ? '-error' : isHovered || isConnected ? '-selected' : ''})`}
                    className="transition-all duration-200"
                    onMouseEnter={() => setHoveredEdge(edgeId)}
                    onMouseLeave={() => setHoveredEdge(null)}
                    style={{ cursor: 'pointer' }}
                  />
                  {/* Edge label */}
                  {(isHovered || isConnected) && (
                    <g>
                      <rect
                        x={(source.x + target.x) / 2 - 35}
                        y={(source.y + target.y) / 2 - 12}
                        width={70}
                        height={24}
                        rx={4}
                        fill="#1f2937"
                        stroke="#374151"
                      />
                      <text
                        x={(source.x + target.x) / 2}
                        y={(source.y + target.y) / 2 + 4}
                        textAnchor="middle"
                        className="text-[10px] fill-gray-300 font-mono"
                      >
                        {formatDuration(edge.avgLatency)}
                      </text>
                    </g>
                  )}
                </g>
              );
            })}

            {/* Nodes */}
            {simulatedNodes.map(node => {
              const isSelected = selectedNode === node.id;
              const isHovered = hoveredNode === node.id;
              const hasError = node.errorCount > 0;

              return (
                <g
                  key={node.id}
                  transform={`translate(${node.x}, ${node.y})`}
                  onMouseEnter={() => setHoveredNode(node.id)}
                  onMouseLeave={() => setHoveredNode(null)}
                  onClick={() => setSelectedNode(isSelected ? null : node.id)}
                  style={{ cursor: 'pointer' }}
                >
                  {/* Outer ring for selection */}
                  {isSelected && (
                    <circle
                      r={node.radius + 8}
                      fill="none"
                      stroke="#3b82f6"
                      strokeWidth={3}
                      opacity={0.5}
                    />
                  )}

                  {/* Main circle */}
                  <circle
                    r={node.radius}
                    fill={node.color}
                    stroke={isHovered || isSelected ? '#ffffff' : '#374151'}
                    strokeWidth={isHovered || isSelected ? 3 : 2}
                    className="transition-all duration-200"
                  />

                  {/* Error indicator */}
                  {hasError && (
                    <g transform={`translate(${node.radius * 0.6}, ${-node.radius * 0.6})`}>
                      <circle r={10} fill="#ef4444" />
                      <text
                        textAnchor="middle"
                        y={4}
                        className="text-[10px] fill-white font-bold"
                      >
                        {node.errorCount}
                      </text>
                    </g>
                  )}

                  {/* Service name */}
                  <text
                    y={node.radius + 16}
                    textAnchor="middle"
                    className="text-xs fill-white font-medium"
                  >
                    {node.name}
                  </text>

                  {/* Span count inside */}
                  <text
                    textAnchor="middle"
                    y={4}
                    className="text-sm fill-white font-bold"
                    style={{ pointerEvents: 'none' }}
                  >
                    {node.spanCount}
                  </text>
                </g>
              );
            })}
          </g>
        </svg>
      </div>

      {/* Selected node details */}
      {selectedNode && (
        <div className="flex-shrink-0 p-3 bg-gray-800 border-t border-gray-700">
          {(() => {
            const node = simulatedNodes.find(n => n.id === selectedNode);
            if (!node) return null;

            const incomingEdges = edges.filter(e => e.target === selectedNode);
            const outgoingEdges = edges.filter(e => e.source === selectedNode);

            return (
              <div className="flex items-start gap-6">
                <div className="flex items-center gap-2">
                  <div
                    className="w-4 h-4 rounded-full"
                    style={{ backgroundColor: node.color }}
                  />
                  <span className="font-medium text-white">{node.name}</span>
                </div>

                <div className="grid grid-cols-4 gap-4 text-sm">
                  <div>
                    <span className="text-gray-400">Spans:</span>
                    <span className="ml-2 text-white">{node.spanCount}</span>
                  </div>
                  <div>
                    <span className="text-gray-400">Avg Duration:</span>
                    <span className="ml-2 text-white font-mono">{formatDuration(node.avgDuration)}</span>
                  </div>
                  <div>
                    <span className="text-gray-400">Incoming:</span>
                    <span className="ml-2 text-white">{incomingEdges.length}</span>
                  </div>
                  <div>
                    <span className="text-gray-400">Outgoing:</span>
                    <span className="ml-2 text-white">{outgoingEdges.length}</span>
                  </div>
                </div>

                {node.errorCount > 0 && (
                  <div className="flex items-center gap-1 text-red-400">
                    <AlertCircle className="w-4 h-4" />
                    <span className="text-sm">{node.errorCount} errors</span>
                  </div>
                )}
              </div>
            );
          })()}
        </div>
      )}

      {/* Legend */}
      <div className="flex-shrink-0 px-3 py-2 bg-gray-800/50 border-t border-gray-700 flex items-center gap-4 text-xs text-gray-400">
        <div className="flex items-center gap-1">
          <div className="w-3 h-3 rounded-full bg-gray-500" />
          <span>Service (span count inside)</span>
        </div>
        <div className="flex items-center gap-1">
          <div className="w-6 h-0.5 bg-gray-500" />
          <span>Request flow</span>
        </div>
        <div className="flex items-center gap-1">
          <div className="w-6 h-0.5 bg-red-500" style={{ backgroundImage: 'repeating-linear-gradient(90deg, #ef4444 0px, #ef4444 5px, transparent 5px, transparent 10px)' }} />
          <span>Error path</span>
        </div>
      </div>
    </div>
  );
}

export default ServiceMap;
