import { useState } from 'react';
import {
  Search,
  Loader2,
  MessageSquare,
  Database,
  Table,
  AlertCircle,
  ChevronRight,
  Sparkles,
} from 'lucide-react';
import { clsx } from 'clsx';
import { getAiEngineUrl } from '../../lib/config';

const AI_ENGINE_URL = getAiEngineUrl();

interface NLQResult {
  success: boolean;
  question: string;
  sql?: string;
  explanation?: string;
  visualization?: string;
  results?: any[];
  count?: number;
  error?: string;
}

const EXAMPLE_QUERIES = [
  "Show me errors by service",
  "Which services are slow?",
  "What's the service health?",
  "Show recent correlations",
  "Top operations by count",
];

export default function NLQueryWidget() {
  const [query, setQuery] = useState('');
  const [loading, setLoading] = useState(false);
  const [result, setResult] = useState<NLQResult | null>(null);
  const [error, setError] = useState<string | null>(null);

  const executeQuery = async (q: string) => {
    if (!q.trim()) return;

    setLoading(true);
    setError(null);
    setResult(null);

    try {
      const response = await fetch(`${AI_ENGINE_URL}/api/v1/nlq`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ question: q }),
      });

      if (!response.ok) {
        throw new Error('Failed to execute query');
      }

      const data: NLQResult = await response.json();
      setResult(data);

      if (!data.success) {
        setError(data.error || 'Query failed');
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to connect to AI Engine');
    } finally {
      setLoading(false);
    }
  };

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    executeQuery(query);
  };

  const handleExampleClick = (example: string) => {
    setQuery(example);
    executeQuery(example);
  };

  const formatValue = (value: any): string => {
    if (value === null || value === undefined) return '-';
    if (typeof value === 'number') {
      return value.toLocaleString();
    }
    if (typeof value === 'string' && value.length > 50) {
      return value.slice(0, 47) + '...';
    }
    return String(value);
  };

  return (
    <div className="bg-gray-800 rounded-lg overflow-hidden">
      {/* Header */}
      <div className="p-4 border-b border-gray-700 bg-gradient-to-r from-indigo-900/30 to-purple-900/30">
        <div className="flex items-center gap-2 mb-3">
          <Sparkles className="w-5 h-5 text-indigo-400" />
          <span className="font-semibold">Natural Language Query</span>
        </div>

        {/* Search Form */}
        <form onSubmit={handleSubmit} className="relative">
          <MessageSquare className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-gray-400" />
          <input
            type="text"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder="Ask a question about your data..."
            className="w-full pl-10 pr-24 py-2.5 bg-gray-700 border border-gray-600 rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-indigo-500 focus:border-transparent"
          />
          <button
            type="submit"
            disabled={loading || !query.trim()}
            className={clsx(
              'absolute right-2 top-1/2 -translate-y-1/2 px-3 py-1.5 rounded text-sm font-medium transition-colors',
              loading || !query.trim()
                ? 'bg-gray-600 text-gray-400 cursor-not-allowed'
                : 'bg-indigo-600 text-white hover:bg-indigo-500'
            )}
          >
            {loading ? (
              <Loader2 className="w-4 h-4 animate-spin" />
            ) : (
              <Search className="w-4 h-4" />
            )}
          </button>
        </form>

        {/* Example Queries */}
        <div className="flex flex-wrap gap-2 mt-3">
          {EXAMPLE_QUERIES.map((example) => (
            <button
              key={example}
              onClick={() => handleExampleClick(example)}
              className="text-xs px-2 py-1 bg-gray-700 hover:bg-gray-600 rounded text-gray-400 hover:text-white transition-colors"
            >
              {example}
            </button>
          ))}
        </div>
      </div>

      {/* Results */}
      <div className="p-4 max-h-96 overflow-auto">
        {error && (
          <div className="flex items-start gap-2 p-3 bg-red-900/20 border border-red-800 rounded-lg text-red-400">
            <AlertCircle className="w-4 h-4 mt-0.5 flex-shrink-0" />
            <div>
              <div className="font-medium">Error</div>
              <div className="text-sm opacity-80">{error}</div>
            </div>
          </div>
        )}

        {result && result.success && (
          <div className="space-y-4">
            {/* Explanation */}
            {result.explanation && (
              <div className="flex items-start gap-2 text-sm text-gray-400">
                <ChevronRight className="w-4 h-4 mt-0.5 flex-shrink-0" />
                <span>{result.explanation}</span>
              </div>
            )}

            {/* SQL Query (collapsible) */}
            {result.sql && (
              <details className="text-sm">
                <summary className="cursor-pointer text-gray-500 hover:text-gray-400 flex items-center gap-1">
                  <Database className="w-3 h-3" />
                  View SQL
                </summary>
                <pre className="mt-2 p-2 bg-gray-900 rounded text-xs text-gray-400 overflow-x-auto">
                  {result.sql}
                </pre>
              </details>
            )}

            {/* Results Table */}
            {result.results && result.results.length > 0 && (
              <div className="overflow-x-auto">
                <div className="flex items-center gap-2 text-sm text-gray-400 mb-2">
                  <Table className="w-4 h-4" />
                  <span>{result.count || result.results.length} results</span>
                </div>
                <table className="w-full text-sm">
                  <thead>
                    <tr className="border-b border-gray-700">
                      {Object.keys(result.results[0]).map((key) => (
                        <th
                          key={key}
                          className="text-left py-2 px-3 text-gray-400 font-medium"
                        >
                          {key}
                        </th>
                      ))}
                    </tr>
                  </thead>
                  <tbody>
                    {result.results.slice(0, 20).map((row, idx) => (
                      <tr
                        key={idx}
                        className="border-b border-gray-700/50 hover:bg-gray-750"
                      >
                        {Object.values(row).map((value, colIdx) => (
                          <td key={colIdx} className="py-2 px-3 text-gray-300">
                            {formatValue(value)}
                          </td>
                        ))}
                      </tr>
                    ))}
                  </tbody>
                </table>
                {result.results.length > 20 && (
                  <div className="text-center text-xs text-gray-500 py-2">
                    Showing 20 of {result.results.length} results
                  </div>
                )}
              </div>
            )}

            {result.results && result.results.length === 0 && (
              <div className="text-center py-6 text-gray-400">
                <Database className="w-8 h-8 mx-auto mb-2 opacity-50" />
                <p className="text-sm">No results found</p>
              </div>
            )}
          </div>
        )}

        {!result && !error && !loading && (
          <div className="text-center py-8 text-gray-400">
            <MessageSquare className="w-8 h-8 mx-auto mb-2 opacity-50" />
            <p className="text-sm">Ask a question about your observability data</p>
            <p className="text-xs mt-1 text-gray-500">
              Try: "Show me errors by service" or "Which services are slow?"
            </p>
          </div>
        )}
      </div>
    </div>
  );
}
