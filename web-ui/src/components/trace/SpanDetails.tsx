import { useMemo, useState } from 'react';
import {
  X,
  Copy,
  Check,
  ChevronDown,
  ChevronRight,
  ExternalLink,
  Clock,
  Server,
  Tag,
  Activity,
  Link2,
  FileText,
  Sparkles,
} from 'lucide-react';
import { useTraceStore } from '../../stores/traceStore';
import { formatDuration, formatTimestamp } from '../../types/trace';
import { clsx } from 'clsx';

type TabId = 'attributes' | 'events' | 'links' | 'logs';

export function SpanDetails() {
  const { trace, selectedSpanId, selectSpan } = useTraceStore();
  const [activeTab, setActiveTab] = useState<TabId>('attributes');
  const [copiedKey, setCopiedKey] = useState<string | null>(null);
  const [expandedSections, setExpandedSections] = useState<Set<string>>(
    new Set(['span', 'resource'])
  );

  const selectedSpan = useMemo(() => {
    if (!trace || !selectedSpanId) return null;
    return trace.spans.find(s => s.spanId === selectedSpanId);
  }, [trace, selectedSpanId]);

  const handleCopy = async (key: string, value: string) => {
    await navigator.clipboard.writeText(value);
    setCopiedKey(key);
    setTimeout(() => setCopiedKey(null), 2000);
  };

  const toggleSection = (section: string) => {
    const newSet = new Set(expandedSections);
    if (newSet.has(section)) {
      newSet.delete(section);
    } else {
      newSet.add(section);
    }
    setExpandedSections(newSet);
  };

  if (!selectedSpan) {
    return (
      <div className="h-full flex flex-col items-center justify-center text-gray-500 p-8">
        <Activity className="w-12 h-12 mb-4 opacity-50" />
        <p className="text-center">Select a span to view details</p>
      </div>
    );
  }

  // Separate attributes into categories
  const spanAttributes = Object.entries(selectedSpan.attributes).filter(
    ([key]) => !key.startsWith('resource.')
  );
  const resourceAttributes = Object.entries(selectedSpan.attributes).filter(
    ([key]) => key.startsWith('resource.')
  );

  const tabs: { id: TabId; label: string; count?: number }[] = [
    { id: 'attributes', label: 'Attributes', count: spanAttributes.length },
    { id: 'events', label: 'Events', count: selectedSpan.events.length },
    { id: 'links', label: 'Links', count: selectedSpan.links.length },
    { id: 'logs', label: 'Logs' },
  ];

  return (
    <div className="h-full flex flex-col bg-gray-900 border-l border-gray-700">
      {/* Header */}
      <div className="flex-shrink-0 border-b border-gray-700">
        <div className="flex items-center justify-between p-3">
          <div className="flex items-center gap-2">
            <div
              className="w-3 h-3 rounded-full"
              style={{ backgroundColor: selectedSpan.serviceColor }}
            />
            <span className="font-medium text-white">{selectedSpan.serviceName}</span>
          </div>
          <button
            onClick={() => selectSpan(null)}
            className="p-1 rounded hover:bg-gray-700 text-gray-400 hover:text-white"
          >
            <X className="w-4 h-4" />
          </button>
        </div>
        <div className="px-3 pb-3">
          <h3 className="text-sm text-gray-300 truncate" title={selectedSpan.operationName}>
            {selectedSpan.operationName}
          </h3>
        </div>
      </div>

      {/* Summary Stats */}
      <div className="flex-shrink-0 grid grid-cols-2 gap-2 p-3 border-b border-gray-700">
        <StatCard
          icon={<Clock className="w-4 h-4" />}
          label="Duration"
          value={formatDuration(selectedSpan.duration)}
        />
        <StatCard
          icon={<Activity className="w-4 h-4" />}
          label="Self Time"
          value={formatDuration(selectedSpan.selfTime)}
        />
        <StatCard
          icon={<Server className="w-4 h-4" />}
          label="Kind"
          value={selectedSpan.kind}
        />
        <StatCard
          icon={<Tag className="w-4 h-4" />}
          label="Status"
          value={selectedSpan.status}
          valueClassName={selectedSpan.status === 'ERROR' ? 'text-red-400' : 'text-green-400'}
        />
      </div>

      {/* IDs */}
      <div className="flex-shrink-0 p-3 border-b border-gray-700 space-y-2">
        <CopyableField
          label="Trace ID"
          value={selectedSpan.traceId}
          onCopy={handleCopy}
          copied={copiedKey === 'traceId'}
        />
        <CopyableField
          label="Span ID"
          value={selectedSpan.spanId}
          onCopy={handleCopy}
          copied={copiedKey === 'spanId'}
        />
        {selectedSpan.parentSpanId && (
          <CopyableField
            label="Parent Span ID"
            value={selectedSpan.parentSpanId}
            onCopy={handleCopy}
            copied={copiedKey === 'parentSpanId'}
          />
        )}
      </div>

      {/* Tabs */}
      <div className="flex-shrink-0 flex border-b border-gray-700">
        {tabs.map(tab => (
          <button
            key={tab.id}
            onClick={() => setActiveTab(tab.id)}
            className={clsx(
              'flex-1 px-3 py-2 text-xs font-medium transition-colors',
              activeTab === tab.id
                ? 'text-blue-400 border-b-2 border-blue-400'
                : 'text-gray-400 hover:text-white'
            )}
          >
            {tab.label}
            {tab.count !== undefined && tab.count > 0 && (
              <span className="ml-1 px-1.5 py-0.5 rounded-full bg-gray-700 text-[10px]">
                {tab.count}
              </span>
            )}
          </button>
        ))}
      </div>

      {/* Tab Content */}
      <div className="flex-1 overflow-auto">
        {activeTab === 'attributes' && (
          <div className="p-2 space-y-2">
            {/* Span Attributes */}
            <CollapsibleSection
              title="Span Attributes"
              count={spanAttributes.length}
              isExpanded={expandedSections.has('span')}
              onToggle={() => toggleSection('span')}
            >
              <AttributeList
                attributes={spanAttributes}
                onCopy={handleCopy}
                copiedKey={copiedKey}
              />
            </CollapsibleSection>

            {/* Resource Attributes */}
            {resourceAttributes.length > 0 && (
              <CollapsibleSection
                title="Resource Attributes"
                count={resourceAttributes.length}
                isExpanded={expandedSections.has('resource')}
                onToggle={() => toggleSection('resource')}
              >
                <AttributeList
                  attributes={resourceAttributes}
                  onCopy={handleCopy}
                  copiedKey={copiedKey}
                />
              </CollapsibleSection>
            )}
          </div>
        )}

        {activeTab === 'events' && (
          <div className="p-2">
            {selectedSpan.events.length === 0 ? (
              <EmptyState icon={<Activity />} message="No events recorded" />
            ) : (
              <div className="space-y-2">
                {selectedSpan.events.map((event, i) => (
                  <div key={i} className="bg-gray-800 rounded-lg p-3">
                    <div className="flex items-center justify-between mb-2">
                      <span className="text-sm font-medium text-white">{event.name}</span>
                      <span className="text-xs text-gray-400">
                        {formatTimestamp(event.timestamp)}
                      </span>
                    </div>
                    {Object.entries(event.attributes).length > 0 && (
                      <AttributeList
                        attributes={Object.entries(event.attributes)}
                        onCopy={handleCopy}
                        copiedKey={copiedKey}
                        compact
                      />
                    )}
                  </div>
                ))}
              </div>
            )}
          </div>
        )}

        {activeTab === 'links' && (
          <div className="p-2">
            {selectedSpan.links.length === 0 ? (
              <EmptyState icon={<Link2 />} message="No span links" />
            ) : (
              <div className="space-y-2">
                {selectedSpan.links.map((link, i) => (
                  <div key={i} className="bg-gray-800 rounded-lg p-3">
                    <div className="flex items-center gap-2 mb-2">
                      <Link2 className="w-4 h-4 text-gray-400" />
                      <span className="text-sm font-medium text-white">Linked Span</span>
                    </div>
                    <div className="space-y-1 text-xs">
                      <div className="flex justify-between">
                        <span className="text-gray-400">Trace ID</span>
                        <span className="text-gray-200 font-mono">{link.traceId.slice(0, 16)}...</span>
                      </div>
                      <div className="flex justify-between">
                        <span className="text-gray-400">Span ID</span>
                        <span className="text-gray-200 font-mono">{link.spanId}</span>
                      </div>
                    </div>
                  </div>
                ))}
              </div>
            )}
          </div>
        )}

        {activeTab === 'logs' && (
          <div className="p-2">
            <EmptyState
              icon={<FileText />}
              message="View correlated logs"
              action={
                <button className="mt-2 px-3 py-1.5 bg-blue-600 hover:bg-blue-700 rounded text-sm text-white flex items-center gap-1">
                  <ExternalLink className="w-3 h-3" />
                  Open in Logs Explorer
                </button>
              }
            />
          </div>
        )}
      </div>

      {/* AI Analysis Button */}
      <div className="flex-shrink-0 p-3 border-t border-gray-700">
        <button className="w-full px-4 py-2 bg-gradient-to-r from-purple-600 to-blue-600 hover:from-purple-700 hover:to-blue-700 rounded-lg text-white text-sm font-medium flex items-center justify-center gap-2 transition-all">
          <Sparkles className="w-4 h-4" />
          Analyze with AI
        </button>
      </div>
    </div>
  );
}

// Stat card component
function StatCard({
  icon,
  label,
  value,
  valueClassName,
}: {
  icon: React.ReactNode;
  label: string;
  value: string;
  valueClassName?: string;
}) {
  return (
    <div className="bg-gray-800 rounded-lg p-2">
      <div className="flex items-center gap-1.5 text-gray-400 mb-1">
        {icon}
        <span className="text-[10px] uppercase">{label}</span>
      </div>
      <div className={clsx('text-sm font-medium', valueClassName || 'text-white')}>
        {value}
      </div>
    </div>
  );
}

// Copyable field component
function CopyableField({
  label,
  value,
  onCopy,
  copied,
}: {
  label: string;
  value: string;
  onCopy: (key: string, value: string) => void;
  copied: boolean;
}) {
  return (
    <div className="flex items-center justify-between">
      <span className="text-xs text-gray-400">{label}</span>
      <div className="flex items-center gap-1">
        <span className="text-xs text-gray-200 font-mono">{value.slice(0, 16)}...</span>
        <button
          onClick={() => onCopy(label, value)}
          className="p-1 rounded hover:bg-gray-700 text-gray-400 hover:text-white"
        >
          {copied ? (
            <Check className="w-3 h-3 text-green-400" />
          ) : (
            <Copy className="w-3 h-3" />
          )}
        </button>
      </div>
    </div>
  );
}

// Collapsible section component
function CollapsibleSection({
  title,
  count,
  isExpanded,
  onToggle,
  children,
}: {
  title: string;
  count: number;
  isExpanded: boolean;
  onToggle: () => void;
  children: React.ReactNode;
}) {
  return (
    <div className="bg-gray-800 rounded-lg overflow-hidden">
      <button
        onClick={onToggle}
        className="w-full flex items-center justify-between p-2 hover:bg-gray-700/50 transition-colors"
      >
        <div className="flex items-center gap-2">
          {isExpanded ? (
            <ChevronDown className="w-4 h-4 text-gray-400" />
          ) : (
            <ChevronRight className="w-4 h-4 text-gray-400" />
          )}
          <span className="text-sm font-medium text-white">{title}</span>
          <span className="px-1.5 py-0.5 rounded-full bg-gray-700 text-[10px] text-gray-400">
            {count}
          </span>
        </div>
      </button>
      {isExpanded && <div className="px-2 pb-2">{children}</div>}
    </div>
  );
}

// Attribute list component
function AttributeList({
  attributes,
  onCopy,
  copiedKey,
  compact = false,
}: {
  attributes: [string, string | number | boolean][];
  onCopy: (key: string, value: string) => void;
  copiedKey: string | null;
  compact?: boolean;
}) {
  return (
    <div className={clsx('space-y-1', compact && 'text-xs')}>
      {attributes.map(([key, value]) => (
        <div
          key={key}
          className="flex items-start justify-between gap-2 py-1 border-b border-gray-700/50 last:border-0"
        >
          <span className="text-gray-400 truncate flex-shrink-0" style={{ maxWidth: '40%' }}>
            {key.replace('resource.', '')}
          </span>
          <div className="flex items-center gap-1 min-w-0">
            <span className="text-gray-200 font-mono truncate text-right">
              {String(value)}
            </span>
            <button
              onClick={() => onCopy(key, String(value))}
              className="p-0.5 rounded hover:bg-gray-600 text-gray-500 hover:text-white flex-shrink-0"
            >
              {copiedKey === key ? (
                <Check className="w-3 h-3 text-green-400" />
              ) : (
                <Copy className="w-3 h-3" />
              )}
            </button>
          </div>
        </div>
      ))}
    </div>
  );
}

// Empty state component
function EmptyState({
  icon,
  message,
  action,
}: {
  icon: React.ReactNode;
  message: string;
  action?: React.ReactNode;
}) {
  return (
    <div className="flex flex-col items-center justify-center py-8 text-gray-500">
      <div className="w-10 h-10 rounded-full bg-gray-800 flex items-center justify-center mb-3">
        {icon}
      </div>
      <p className="text-sm">{message}</p>
      {action}
    </div>
  );
}

export default SpanDetails;
