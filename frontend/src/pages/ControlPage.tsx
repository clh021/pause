import { useCallback, useEffect, useRef, useState } from 'react';
import { forceBreak, forceUnlock, getScreenshotUrl } from '../api';
import { t, type Locale } from '../i18n';
import type { RuntimeState } from '../types';

type ControlPageProps = {
  locale: Locale;
  runtime: RuntimeState | null;
  onRuntimeRefresh: () => void;
};

export function ControlPage({ locale, runtime, onRuntimeRefresh }: ControlPageProps) {
  const [screenshotKey, setScreenshotKey] = useState(0);
  const [screenshotError, setScreenshotError] = useState(false);
  const [actionLoading, setActionLoading] = useState<string | null>(null);
  const [actionResult, setActionResult] = useState<{ key: string; message: string; ok: boolean } | null>(null);
  const resultTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  const refreshScreenshot = useCallback(() => {
    setScreenshotError(false);
    setScreenshotKey((prev) => prev + 1);
  }, []);

  const handleForceBreak = useCallback(async () => {
    setActionLoading('forceBreak');
    try {
      await forceBreak();
      setActionResult({ key: 'forceBreak', message: t(locale, 'controlBreakDone'), ok: true });
    } catch (err) {
      setActionResult({ key: 'forceBreak', message: String(err), ok: false });
    } finally {
      setActionLoading(null);
      onRuntimeRefresh();
    }
  }, [locale, onRuntimeRefresh]);

  const handleForceUnlock = useCallback(async () => {
    setActionLoading('forceUnlock');
    try {
      await forceUnlock();
      setActionResult({ key: 'forceUnlock', message: t(locale, 'controlUnlockDone'), ok: true });
    } catch (err) {
      setActionResult({ key: 'forceUnlock', message: String(err), ok: false });
    } finally {
      setActionLoading(null);
      onRuntimeRefresh();
    }
  }, [locale, onRuntimeRefresh]);

  useEffect(() => {
    if (resultTimerRef.current) {
      clearTimeout(resultTimerRef.current);
    }
    if (actionResult) {
      resultTimerRef.current = setTimeout(() => setActionResult(null), 4000);
    }
    return () => {
      if (resultTimerRef.current) clearTimeout(resultTimerRef.current);
    };
  }, [actionResult]);

  const isResting = runtime?.currentSession?.status === 'resting';

  const actionBtnClass = (loading: boolean, danger?: boolean) =>
    `inline-flex items-center justify-center gap-2 rounded-lg border-0 px-5 py-3 text-sm font-medium transition-all duration-150 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--control-focus-ring)] disabled:opacity-40 ${
      loading
        ? 'cursor-wait'
        : 'cursor-pointer'
    } ${
      danger
        ? 'bg-[var(--danger-bg,#e81123)] text-white hover:bg-[var(--danger-hover-bg,#c50f1f)]'
        : 'bg-[var(--accent-bg)] text-white hover:brightness-110'
    }`;

  return (
    <section className="mt-3 space-y-6 px-2 pb-4 sm:px-3">
      {/* Current status */}
      <div className="flex items-center gap-3 rounded-xl border border-[var(--card-border)] bg-[var(--card-bg)] px-4 py-3 shadow-[var(--shadow-subtle)]">
        <div
          className={`h-3 w-3 shrink-0 rounded-full ${
            isResting ? 'bg-[#f59e0b]' : runtime?.globalEnabled ? 'bg-[#22c55e]' : 'bg-[#6b7280]'
          }`}
        />
        <span className="text-sm text-[var(--text-primary)]">
          {isResting
            ? `${t(locale, 'statusOnBreak')} — ${runtime?.currentSession?.remainingSec ?? 0}s ${t(locale, 'remaining')}`
            : runtime?.globalEnabled
              ? t(locale, 'statusRunning')
              : t(locale, 'statusDisabled')}
        </span>
      </div>

      {/* Quick actions */}
      <div className="rounded-xl border border-[var(--card-border)] bg-[var(--card-bg)] p-4 shadow-[var(--shadow-subtle)]">
        <h2 className="mb-3 text-sm font-semibold text-[var(--text-primary)]">{t(locale, 'controlActions')}</h2>
        <div className="flex flex-wrap gap-3">
          <button
            type="button"
            className={actionBtnClass(actionLoading === 'forceBreak')}
            disabled={actionLoading !== null}
            onClick={() => void handleForceBreak()}
          >
            {actionLoading === 'forceBreak' ? t(locale, 'loading') : t(locale, 'controlForceBreak')}
          </button>
          <button
            type="button"
            className={actionBtnClass(actionLoading === 'forceUnlock', true)}
            disabled={actionLoading !== null || !isResting}
            onClick={() => void handleForceUnlock()}
          >
            {actionLoading === 'forceUnlock' ? t(locale, 'loading') : t(locale, 'controlForceUnlock')}
          </button>
        </div>
        {actionResult && (
          <p
            className={`mt-3 text-xs ${
              actionResult.ok ? 'text-[#22c55e]' : 'text-[var(--danger-bg,#e81123)]'
            }`}
          >
            {actionResult.message}
          </p>
        )}
      </div>

      {/* Screenshot */}
      <div className="rounded-xl border border-[var(--card-border)] bg-[var(--card-bg)] p-4 shadow-[var(--shadow-subtle)]">
        <div className="mb-3 flex items-center justify-between">
          <h2 className="text-sm font-semibold text-[var(--text-primary)]">{t(locale, 'controlScreenshot')}</h2>
          <button
            type="button"
            className="inline-flex cursor-pointer items-center gap-1 rounded-md border-0 bg-transparent px-3 py-1 text-xs font-medium text-[var(--accent-bg)] transition-colors hover:bg-[var(--seg-hover-bg)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--control-focus-ring)]"
            onClick={refreshScreenshot}
          >
            {t(locale, 'controlScreenshotRefresh')}
          </button>
        </div>
        <div className="relative overflow-hidden rounded-lg border border-[var(--card-border)] bg-[var(--seg-bg)]">
          {screenshotError ? (
            <div className="flex items-center justify-center py-16 text-sm text-[var(--text-secondary)]">
              {t(locale, 'controlScreenshotError')}
            </div>
          ) : (
            <img
              key={screenshotKey}
              src={`${getScreenshotUrl()}?t=${Date.now()}`}
              alt={t(locale, 'controlScreenshot')}
              className="block w-full max-h-[70vh] object-contain"
              onError={() => setScreenshotError(true)}
            />
          )}
        </div>
      </div>
    </section>
  );
}
