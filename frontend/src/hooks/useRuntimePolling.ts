import { useCallback, useEffect, useRef, useState } from 'react';
import { getRuntimeState } from '../api';
import type { RuntimeState } from '../types';

type UseRuntimePollingOptions = {
  setError: (message: string) => void;
  setBootstrapError: (message: string) => void;
  clearError: () => void;
};

export function useRuntimePolling({ setError, setBootstrapError, clearError }: UseRuntimePollingOptions) {
  const [runtime, setRuntime] = useState<RuntimeState | null>(null);
  const mountedRef = useRef(false);
  const hasLoadedRuntimeRef = useRef(false);
  const lastReportedErrorRef = useRef('');
  const runtimeRef = useRef<RuntimeState | null>(null);

  const refreshRuntime = useCallback(async (): Promise<RuntimeState | null> => {
    try {
      const state = await getRuntimeState();
      if (mountedRef.current) {
        setRuntime(state);
        runtimeRef.current = state;
        setBootstrapError('');
        if (lastReportedErrorRef.current !== '') {
          lastReportedErrorRef.current = '';
          clearError();
        }
      }
      hasLoadedRuntimeRef.current = true;
      return state;
    } catch (err) {
      const message = String(err);
      if (mountedRef.current) {
        if (!hasLoadedRuntimeRef.current) {
          setBootstrapError(message);
        } else if (lastReportedErrorRef.current !== message) {
          lastReportedErrorRef.current = message;
          setError(message);
        }
      }
      return null;
    }
  }, [clearError, setBootstrapError, setError]);

  useEffect(() => {
    mountedRef.current = true;
    let timer: number | null = null;

    const nextDelayMs = () => {
      return runtimeRef.current?.currentSession?.status === 'resting' ? 1000 : 5000;
    };

    const scheduleNextPoll = () => {
      if (timer !== null || document.visibilityState !== 'visible') {
        return;
      }
      timer = window.setTimeout(async () => {
        timer = null;
        await refreshRuntime();
        scheduleNextPoll();
      }, nextDelayMs());
    };

    const stopPolling = () => {
      if (timer === null) return;
      window.clearTimeout(timer);
      timer = null;
    };

    const handleVisibilityChange = () => {
      if (document.visibilityState === 'visible') {
        void refreshRuntime().finally(() => {
          scheduleNextPoll();
        });
      } else {
        stopPolling();
      }
    };

    void refreshRuntime().finally(() => {
      scheduleNextPoll();
    });
    document.addEventListener('visibilitychange', handleVisibilityChange);

    return () => {
      mountedRef.current = false;
      document.removeEventListener('visibilitychange', handleVisibilityChange);
      stopPolling();
    };
  }, [refreshRuntime]);

  return {
    runtime,
    setRuntime,
    refreshRuntime,
    resetReportedError: () => {
      lastReportedErrorRef.current = '';
    }
  };
}
