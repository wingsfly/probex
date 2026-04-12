import { useEffect, useRef } from 'react';
import { useQueryClient } from '@tanstack/react-query';

/**
 * useSSE subscribes to the server-sent events stream and invalidates
 * relevant React Query caches when events arrive.
 */
export function useSSE(enabled: boolean = true) {
  const queryClient = useQueryClient();
  const esRef = useRef<EventSource | null>(null);

  useEffect(() => {
    if (!enabled) return;

    const es = new EventSource('/api/v1/stream');
    esRef.current = es;

    es.onmessage = (event) => {
      try {
        const data = JSON.parse(event.data);
        switch (data.type) {
          case 'result':
            queryClient.invalidateQueries({ queryKey: ['latestResults'] });
            queryClient.invalidateQueries({ queryKey: ['recentResults'] });
            break;
          case 'node_status':
            queryClient.invalidateQueries({ queryKey: ['agents'] });
            break;
          case 'alert':
            queryClient.invalidateQueries({ queryKey: ['alertRules'] });
            queryClient.invalidateQueries({ queryKey: ['alertEvents'] });
            break;
        }
      } catch {
        // ignore parse errors
      }
    };

    es.onerror = () => {
      // EventSource auto-reconnects
    };

    return () => {
      es.close();
      esRef.current = null;
    };
  }, [enabled, queryClient]);
}
