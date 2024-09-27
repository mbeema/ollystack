import { useState, useRef, useEffect } from 'react';
import { X, Send, Sparkles, Database, MessageSquare } from 'lucide-react';
import { motion, AnimatePresence } from 'framer-motion';
import { getAiEngineUrl } from '../lib/config';

const AI_ENGINE_URL = getAiEngineUrl();

interface Message {
  id: string;
  role: 'user' | 'assistant';
  content: string;
  timestamp: Date;
  type?: 'text' | 'query' | 'table';
  data?: any;
}

interface AICopilotProps {
  isOpen: boolean;
  onClose: () => void;
}

export default function AICopilot({ isOpen, onClose }: AICopilotProps) {
  const [messages, setMessages] = useState<Message[]>([
    {
      id: '1',
      role: 'assistant',
      content: "Hi! I'm your OllyStack Copilot. I can help you analyze traces, investigate issues, and understand your system behavior. Try asking me things like:\n\n- \"Why was checkout slow yesterday?\"\n- \"Show me error rates for the payment service\"\n- \"What services are affected by high latency?\"",
      timestamp: new Date(),
    },
  ]);
  const [input, setInput] = useState('');
  const [isLoading, setIsLoading] = useState(false);
  const messagesEndRef = useRef<HTMLDivElement>(null);

  const scrollToBottom = () => {
    messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' });
  };

  useEffect(() => {
    scrollToBottom();
  }, [messages]);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!input.trim() || isLoading) return;

    const userMessage: Message = {
      id: Date.now().toString(),
      role: 'user',
      content: input.trim(),
      timestamp: new Date(),
    };

    setMessages((prev) => [...prev, userMessage]);
    const question = input.trim();
    setInput('');
    setIsLoading(true);

    try {
      // Detect if this is a query request (starts with "query:", "sql:", or "show me")
      const isQuery = /^(query:|sql:|show me|list|find|get|what are|how many)/i.test(question);

      if (isQuery) {
        // Use NLQ endpoint for data queries
        const response = await fetch(`${AI_ENGINE_URL}/api/v1/nlq`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ question }),
        });

        const data = await response.json();

        if (data.success && data.results && data.results.length > 0) {
          const assistantMessage: Message = {
            id: (Date.now() + 1).toString(),
            role: 'assistant',
            content: `**${data.explanation}**\n\nFound ${data.count} results:`,
            timestamp: new Date(),
            type: 'table',
            data: data.results.slice(0, 10),
          };
          setMessages((prev) => [...prev, assistantMessage]);
        } else {
          const assistantMessage: Message = {
            id: (Date.now() + 1).toString(),
            role: 'assistant',
            content: data.error || 'No results found for your query.',
            timestamp: new Date(),
          };
          setMessages((prev) => [...prev, assistantMessage]);
        }
      } else {
        // Use chat endpoint for general questions
        const response = await fetch(`${AI_ENGINE_URL}/api/v1/chat`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ message: question, include_context: true }),
        });

        const data = await response.json();

        const assistantMessage: Message = {
          id: (Date.now() + 1).toString(),
          role: 'assistant',
          content: data.response || "I'm analyzing your data...",
          timestamp: new Date(),
        };

        setMessages((prev) => [...prev, assistantMessage]);
      }
    } catch (error) {
      const errorMessage: Message = {
        id: (Date.now() + 1).toString(),
        role: 'assistant',
        content: "I'm sorry, I encountered an error connecting to the AI engine. Please try again.",
        timestamp: new Date(),
      };
      setMessages((prev) => [...prev, errorMessage]);
    } finally {
      setIsLoading(false);
    }
  };

  const suggestions = [
    "What's causing high latency?",
    "Show me recent errors",
    "Analyze the checkout flow",
    "Find anomalies in the last hour",
  ];

  return (
    <AnimatePresence>
      {isOpen && (
        <motion.div
          initial={{ x: '100%' }}
          animate={{ x: 0 }}
          exit={{ x: '100%' }}
          transition={{ type: 'spring', damping: 25, stiffness: 300 }}
          className="fixed inset-y-0 right-0 w-[450px] bg-gray-800 border-l border-gray-700 shadow-xl z-50 flex flex-col"
        >
          {/* Header */}
          <div className="flex items-center justify-between px-4 py-3 border-b border-gray-700">
            <div className="flex items-center">
              <Sparkles className="w-5 h-5 text-purple-400 mr-2" />
              <span className="font-semibold">AI Copilot</span>
            </div>
            <button
              onClick={onClose}
              className="p-1 hover:bg-gray-700 rounded transition-colors"
            >
              <X className="w-5 h-5" />
            </button>
          </div>

          {/* Messages */}
          <div className="flex-1 overflow-y-auto p-4 space-y-4">
            {messages.map((message) => (
              <div
                key={message.id}
                className={`flex ${message.role === 'user' ? 'justify-end' : 'justify-start'}`}
              >
                <div
                  className={`max-w-[85%] rounded-lg px-4 py-2 ${
                    message.role === 'user'
                      ? 'bg-blue-600 text-white'
                      : 'bg-gray-700 text-gray-100'
                  }`}
                >
                  <p className="text-sm whitespace-pre-wrap">{message.content}</p>
                  {message.type === 'table' && message.data && (
                    <div className="mt-2 overflow-x-auto">
                      <table className="min-w-full text-xs">
                        <thead>
                          <tr className="border-b border-gray-600">
                            {Object.keys(message.data[0] || {}).map((key) => (
                              <th key={key} className="px-2 py-1 text-left text-gray-400">
                                {key}
                              </th>
                            ))}
                          </tr>
                        </thead>
                        <tbody>
                          {message.data.map((row: any, idx: number) => (
                            <tr key={idx} className="border-b border-gray-600/50">
                              {Object.values(row).map((val: any, vidx: number) => (
                                <td key={vidx} className="px-2 py-1">
                                  {typeof val === 'number' ? val.toLocaleString() : String(val)}
                                </td>
                              ))}
                            </tr>
                          ))}
                        </tbody>
                      </table>
                    </div>
                  )}
                </div>
              </div>
            ))}
            {isLoading && (
              <div className="flex justify-start">
                <div className="bg-gray-700 rounded-lg px-4 py-2">
                  <div className="flex space-x-1">
                    <div className="w-2 h-2 bg-gray-400 rounded-full animate-bounce" />
                    <div className="w-2 h-2 bg-gray-400 rounded-full animate-bounce delay-100" />
                    <div className="w-2 h-2 bg-gray-400 rounded-full animate-bounce delay-200" />
                  </div>
                </div>
              </div>
            )}
            <div ref={messagesEndRef} />
          </div>

          {/* Suggestions */}
          {messages.length === 1 && (
            <div className="px-4 pb-2">
              <p className="text-xs text-gray-400 mb-2">Suggestions:</p>
              <div className="flex flex-wrap gap-2">
                {suggestions.map((suggestion) => (
                  <button
                    key={suggestion}
                    onClick={() => setInput(suggestion)}
                    className="text-xs px-3 py-1.5 bg-gray-700 hover:bg-gray-600 rounded-full transition-colors"
                  >
                    {suggestion}
                  </button>
                ))}
              </div>
            </div>
          )}

          {/* Input */}
          <form onSubmit={handleSubmit} className="p-4 border-t border-gray-700">
            <div className="flex items-center space-x-2">
              <input
                type="text"
                value={input}
                onChange={(e) => setInput(e.target.value)}
                placeholder="Ask anything about your system..."
                className="flex-1 px-4 py-2 bg-gray-700 border border-gray-600 rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-purple-500 focus:border-transparent"
                disabled={isLoading}
              />
              <button
                type="submit"
                disabled={isLoading || !input.trim()}
                className="p-2 bg-purple-600 hover:bg-purple-700 disabled:bg-gray-600 disabled:cursor-not-allowed rounded-lg transition-colors"
              >
                <Send className="w-5 h-5" />
              </button>
            </div>
          </form>
        </motion.div>
      )}
    </AnimatePresence>
  );
}
