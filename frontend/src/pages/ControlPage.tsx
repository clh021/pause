import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import {
  forceBreak,
  forceUnlock,
  getActivity,
  getAutoScreenshot,
  getRemoteAssetAccess,
  getShotUrl,
  isRemoteWebMode,
  setAutoScreenshot,
  takeScreenshot
} from '../api';
import { t, type Locale } from '../i18n';
import type { ActivityMinute, ActivitySummary, RuntimeState } from '../types';
import type { RemoteAssetAccess } from '../api';

type ControlPageProps = {
  locale: Locale;
  runtime: RuntimeState | null;
  onRuntimeRefresh: () => void;
};

type PreviewState = {
  name: string;
  minuteStartSec: number;
};

function chunkByHour(minutes: ActivityMinute[]): ActivityMinute[][] {
  const hours: ActivityMinute[][] = [];
  for (let i = 0; i < minutes.length; i += 60) {
    hours.push(minutes.slice(i, i + 60));
  }
  return hours;
}

function formatHourLabel(locale: Locale, minuteStartSec: number): string {
  return new Date(minuteStartSec * 1000).toLocaleString(locale, {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit'
  });
}

function formatMinuteLabel(locale: Locale, minuteStartSec: number): string {
  return new Date(minuteStartSec * 1000).toLocaleString(locale, {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit'
  });
}

function normalizePositiveInteger(value: string): string {
  return value.replace(/\D+/g, '');
}

export function ControlPage({ locale, runtime, onRuntimeRefresh }: ControlPageProps) {
  const [activity, setActivity] = useState<ActivitySummary | null>(null);
  const [autoShot, setAutoShot] = useState(false);
  const [selectedShot, setSelectedShot] = useState<PreviewState | null>(null);
  const [loading, setLoading] = useState(true);
  const [actionLoading, setActionLoading] = useState<string | null>(null);
  const [actionMsg, setActionMsg] = useState<{ key: string; text: string; ok: boolean } | null>(null);
  const [manualPreviewUrl, setManualPreviewUrl] = useState<string | null>(null);
  const [forceBreakMinutes, setForceBreakMinutes] = useState('5');
  const [assetAccess, setAssetAccess] = useState<RemoteAssetAccess | null>(() =>
    isRemoteWebMode() ? { baseUrl: '', accessToken: '' } : null
  );
  const [previewBroken, setPreviewBroken] = useState(false);
  const msgTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const pollTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const manualPreviewUrlRef = useRef<string | null>(null);

  useEffect(() => {
    return () => {
      if (manualPreviewUrlRef.current) {
        URL.revokeObjectURL(manualPreviewUrlRef.current);
      }
      if (msgTimer.current) {
        clearTimeout(msgTimer.current);
      }
      if (pollTimer.current) {
        clearTimeout(pollTimer.current);
      }
    };
  }, []);

  const showMsg = useCallback((key: string, text: string, ok: boolean) => {
    setActionMsg({ key, text, ok });
    if (msgTimer.current) clearTimeout(msgTimer.current);
    msgTimer.current = setTimeout(() => setActionMsg(null), 4000);
  }, []);

  const fetchActivity = useCallback(async () => {
    setLoading(true);
    try {
      const data = await getActivity();
      setActivity(data);
      return data;
    } catch {
      return null;
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    const init = async () => {
      await fetchActivity();
      try {
        setAssetAccess(await getRemoteAssetAccess());
      } catch {
        // Keep preview disabled if remote asset access is unavailable.
      }
      try {
        setAutoShot(await getAutoScreenshot());
      } catch {
        // Ignore if the activity recorder is unavailable.
      }
    };
    void init();
  }, [fetchActivity]);

  useEffect(() => {
    const stopPolling = () => {
      if (pollTimer.current) {
        clearTimeout(pollTimer.current);
        pollTimer.current = null;
      }
    };
    const schedulePolling = () => {
      stopPolling();
      if (document.visibilityState !== 'visible') {
        return;
      }
      pollTimer.current = setTimeout(async () => {
        pollTimer.current = null;
        try {
          await fetchActivity();
        } finally {
          schedulePolling();
        }
      }, 20000);
    };
    const onVisibilityChange = () => {
      if (document.visibilityState === 'visible') {
        void fetchActivity().finally(schedulePolling);
      } else {
        stopPolling();
      }
    };

    schedulePolling();
    document.addEventListener('visibilitychange', onVisibilityChange);
    return () => {
      document.removeEventListener('visibilitychange', onVisibilityChange);
      stopPolling();
    };
  }, [fetchActivity]);

  const handleManualScreenshot = useCallback(async () => {
    setActionLoading('manualShot');
    try {
      const blob = await takeScreenshot();
      const url = URL.createObjectURL(blob);
      if (manualPreviewUrlRef.current) {
        URL.revokeObjectURL(manualPreviewUrlRef.current);
      }
      manualPreviewUrlRef.current = url;
      setManualPreviewUrl(url);
      showMsg('manualShot', t(locale, 'controlShotDone'), true);
      await fetchActivity();
    } catch (err) {
      showMsg('manualShot', String(err), false);
    } finally {
      setActionLoading(null);
    }
  }, [fetchActivity, locale, showMsg]);

  const handleForceBreak = useCallback(async () => {
    const minutes = Number.parseInt(forceBreakMinutes, 10);
    if (!Number.isSafeInteger(minutes) || minutes <= 0) {
      showMsg('forceBreak', t(locale, 'controlForceBreakInvalid'), false);
      return;
    }
    setActionLoading('forceBreak');
    try {
      await forceBreak({ minutes });
      showMsg('forceBreak', t(locale, 'controlBreakDone'), true);
      await onRuntimeRefresh();
    } catch (err) {
      showMsg('forceBreak', String(err), false);
    } finally {
      setActionLoading(null);
    }
  }, [forceBreakMinutes, locale, onRuntimeRefresh, showMsg]);

  const handleForceUnlock = useCallback(async () => {
    setActionLoading('forceUnlock');
    try {
      await forceUnlock();
      showMsg('forceUnlock', t(locale, 'controlUnlockDone'), true);
      await onRuntimeRefresh();
    } catch (err) {
      showMsg('forceUnlock', String(err), false);
    } finally {
      setActionLoading(null);
    }
  }, [locale, onRuntimeRefresh, showMsg]);

  const handleAutoShotToggle = useCallback(async () => {
    const next = !autoShot;
    try {
      const result = await setAutoScreenshot(next);
      setAutoShot(result);
      showMsg('autoShot', result ? t(locale, 'controlAutoShotOn') : t(locale, 'controlAutoShotOff'), true);
    } catch (err) {
      showMsg('autoShot', String(err), false);
    }
  }, [autoShot, locale, showMsg]);

  const groupedHours = useMemo(() => {
    return chunkByHour(activity?.minutes ?? []);
  }, [activity]);

  const isResting = runtime?.currentSession?.status === 'resting';
  const btnBase =
    'inline-flex cursor-pointer items-center justify-center gap-2 rounded-lg border border-transparent px-4 py-2.5 text-sm font-medium transition-colors duration-150 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--control-focus-ring)] disabled:cursor-not-allowed disabled:opacity-45';
  const btnPrimary = `${btnBase} bg-[var(--accent-bg)] text-white hover:brightness-110`;
  const btnDanger = `${btnBase} bg-[var(--danger-bg)] text-white hover:bg-[var(--danger-bg-hover)]`;
  const btnGhost = `${btnBase} border-[var(--card-border)] bg-[var(--card-bg)] text-[var(--text-primary)] hover:bg-[var(--seg-hover-bg)]`;

  return (
    <section className="mt-3 space-y-5 px-2 pb-4 sm:px-3">
      <div className="flex items-center justify-between gap-3 rounded-xl border border-[var(--card-border)] bg-[var(--card-bg)] px-4 py-3 shadow-[var(--shadow-subtle)]">
        <div className="flex items-center gap-3">
          <div
            className={`h-3 w-3 shrink-0 rounded-full ${
              isResting ? 'bg-[var(--status-resting)]' : runtime?.globalEnabled ? 'bg-[var(--status-running)]' : 'bg-[var(--status-disabled)]'
            }`}
          />
          <span className="text-sm text-[var(--text-primary)]">
            {isResting
              ? `${t(locale, 'statusOnBreak')} · ${runtime?.currentSession?.remainingSec ?? 0}s ${t(locale, 'remaining')}`
              : runtime?.globalEnabled
                ? t(locale, 'statusRunning')
                : t(locale, 'statusDisabled')}
          </span>
        </div>
        {actionMsg && (
          <span className={`text-xs ${actionMsg.ok ? 'text-[var(--positive-text)]' : 'text-[var(--negative-text)]'}`}>
            {actionMsg.text}
          </span>
        )}
      </div>

      <div className="space-y-4">
        <div className="rounded-xl border border-[var(--card-border)] bg-[var(--card-bg)] p-4 shadow-[var(--shadow-subtle)]">
          <div className="mb-4 flex flex-wrap items-start justify-between gap-3">
            <div>
              <h2 className="text-sm font-semibold text-[var(--text-primary)]">{t(locale, 'controlActions')}</h2>
              <p className="mt-1 text-xs text-[var(--text-secondary)]">{t(locale, 'controlForceBreakHint')}</p>
            </div>
            <button
              type="button"
              className={btnGhost}
              disabled={actionLoading !== null}
              onClick={() => void fetchActivity()}
            >
              {t(locale, 'controlRefresh')}
            </button>
          </div>

          <div className="grid gap-3 lg:grid-cols-[minmax(0,1.1fr)_auto_auto]">
            <label className="flex flex-col gap-2">
              <span className="text-xs font-medium text-[var(--text-secondary)]">{t(locale, 'controlForceBreakMinutes')}</span>
              <input
                type="text"
                inputMode="numeric"
                className="h-11 rounded-lg border border-[var(--dialog-field-border)] bg-[var(--dialog-field-bg)] px-3 text-sm text-[var(--dialog-field-text)] outline-none transition-colors focus:border-[var(--dialog-field-focus-border)] focus:ring-2 focus:ring-[var(--control-focus-ring)]"
                value={forceBreakMinutes}
                onChange={(event) => setForceBreakMinutes(normalizePositiveInteger(event.target.value))}
              />
            </label>
            <button
              type="button"
              className={`${btnPrimary} min-h-11 ${actionLoading === 'forceBreak' ? 'cursor-wait' : ''}`}
              disabled={actionLoading !== null}
              onClick={() => void handleForceBreak()}
            >
              {t(locale, 'controlForceBreak')}
            </button>
            <button
              type="button"
              className={`${btnDanger} min-h-11 ${actionLoading === 'forceUnlock' ? 'cursor-wait' : ''}`}
              disabled={actionLoading !== null || !isResting}
              onClick={() => void handleForceUnlock()}
            >
              {t(locale, 'controlForceUnlock')}
            </button>
          </div>

          <div className="mt-3 flex flex-wrap items-center gap-3">
            <button
              type="button"
              className={`${btnGhost} ${actionLoading === 'manualShot' ? 'cursor-wait' : ''}`}
              disabled={actionLoading !== null}
              onClick={() => void handleManualScreenshot()}
            >
              {t(locale, 'controlManualShot')}
            </button>
            <label className="flex cursor-pointer items-center gap-3 rounded-lg border border-[var(--card-border)] bg-[var(--surface-muted)] px-3 py-2">
              <input
                type="checkbox"
                className="h-4 w-4 accent-[var(--accent-bg)]"
                checked={autoShot}
                onChange={() => void handleAutoShotToggle()}
              />
              <span className="text-sm text-[var(--text-primary)]">{t(locale, 'controlAutoShot')}</span>
            </label>
          </div>
        </div>

        <div className="rounded-xl border border-[var(--card-border)] bg-[var(--card-bg)] p-4 shadow-[var(--shadow-subtle)]">
          <div className="mb-4 flex flex-wrap items-start justify-between gap-3">
            <div>
              <h2 className="text-sm font-semibold text-[var(--text-primary)]">{t(locale, 'controlTimeline')}</h2>
              <p className="mt-1 text-xs text-[var(--text-secondary)]">
                {activity
                  ? `${activity.activeMinutes}m ${t(locale, 'controlActive')} / ${activity.idleMinutes}m ${t(locale, 'controlIdle')}`
                  : t(locale, 'loading')}
              </p>
            </div>
            <div className="flex flex-wrap items-center gap-3 text-xs text-[var(--text-secondary)]">
              <span className="inline-flex items-center gap-2">
                <span className="h-3 w-3 rounded-full border border-[var(--control-dot-idle-border)] bg-transparent" />
                {t(locale, 'controlLegendIdle')}
              </span>
              <span className="inline-flex items-center gap-2">
                <span className="h-3 w-3 rounded-full border border-[var(--control-dot-active)] bg-[var(--control-dot-active)]" />
                {t(locale, 'controlLegendActive')}
              </span>
              <span className="inline-flex items-center gap-2">
                <span className="h-3 w-3 rounded-full border border-[var(--control-dot-shot-border)] bg-[var(--control-dot-active)] ring-1 ring-[var(--control-dot-shot-border)] ring-offset-1 ring-offset-[var(--card-bg)]" />
                {t(locale, 'controlLegendScreenshot')}
              </span>
            </div>
          </div>

          {loading && groupedHours.length === 0 ? (
            <p className="py-6 text-center text-sm text-[var(--text-secondary)]">{t(locale, 'loading')}</p>
          ) : groupedHours.length === 0 ? (
            <p className="py-6 text-center text-sm text-[var(--text-secondary)]">{t(locale, 'controlTimelineEmpty')}</p>
          ) : (
            <div className="space-y-3">
              {groupedHours.map((hourMinutes) => {
                const hourStart = hourMinutes[0]?.minuteStartSec ?? 0;
                return (
                  <article
                    key={hourStart}
                    className="rounded-lg border border-[var(--card-border)] bg-[var(--surface-muted)] p-4 shadow-[var(--shadow-soft)]"
                  >
                    <header className="mb-3 flex items-center justify-between gap-2">
                      <span className="text-xs font-semibold text-[var(--text-primary)]">{formatHourLabel(locale, hourStart)}</span>
                      <span className="text-[11px] text-[var(--text-tertiary)]">{t(locale, 'controlHourLabel')}</span>
                    </header>
                    <div className="grid grid-cols-[repeat(12,minmax(0,1fr))] gap-1.5 md:grid-cols-[repeat(20,minmax(0,1fr))]">
                      {hourMinutes.map((minute) => {
                        const isSelected = selectedShot?.name === minute.shotName;
                        const className = minute.hasScreenshot
                          ? `border-[var(--control-dot-shot-border)] bg-[var(--control-dot-active)] ring-1 ring-[var(--control-dot-shot-border)] ring-offset-1 ring-offset-[var(--surface-muted)] ${
                              isSelected ? 'scale-[1.02] shadow-[0_0_0_1px_var(--control-dot-shot-border)]' : ''
                            }`
                          : minute.active
                            ? 'border-[var(--control-dot-active)] bg-[var(--control-dot-active)]'
                            : 'border-[var(--control-dot-idle-border)] bg-transparent';
                        return (
                          <button
                            key={minute.minuteStartSec}
                            type="button"
                            disabled={!minute.hasScreenshot}
                            className={`aspect-square rounded-full border transition-transform duration-150 ${
                              minute.hasScreenshot ? 'cursor-pointer hover:scale-105' : 'cursor-default'
                            } ${className}`}
                            title={formatMinuteLabel(locale, minute.minuteStartSec)}
                            onClick={() => {
                              if (!minute.shotName) return;
                              setPreviewBroken(false);
                              setSelectedShot({ name: minute.shotName, minuteStartSec: minute.minuteStartSec });
                            }}
                          />
                        );
                      })}
                    </div>
                  </article>
                );
              })}
            </div>
          )}
        </div>

        {manualPreviewUrl && (
          <section className="rounded-xl border border-[var(--card-border)] bg-[var(--card-bg)] p-4 shadow-[var(--shadow-subtle)]">
            <div className="mb-3 flex items-center justify-between gap-3">
              <h3 className="text-sm font-semibold text-[var(--text-primary)]">{t(locale, 'controlManualShot')}</h3>
              <button
                type="button"
                className="rounded-md px-2 py-1 text-xs text-[var(--text-secondary)] hover:bg-[var(--seg-hover-bg)]"
                onClick={() => {
                  if (manualPreviewUrlRef.current) {
                    URL.revokeObjectURL(manualPreviewUrlRef.current);
                    manualPreviewUrlRef.current = null;
                  }
                  setManualPreviewUrl(null);
                }}
              >
                {t(locale, 'close')}
              </button>
            </div>
            <div className="rounded-lg bg-[var(--surface-muted)] p-2">
              <img
                src={manualPreviewUrl}
                alt="manual screenshot"
                className="max-h-[22rem] w-full rounded-lg bg-[var(--surface-muted)] object-contain"
              />
            </div>
          </section>
        )}

        <section className="rounded-xl border border-[var(--card-border)] bg-[var(--card-bg)] p-4 shadow-[var(--shadow-subtle)]">
          <div className="mb-3">
            <h3 className="text-sm font-semibold text-[var(--text-primary)]">{t(locale, 'controlScreenshot')}</h3>
            <p className="mt-1 text-xs text-[var(--text-secondary)]">
              {selectedShot ? formatMinuteLabel(locale, selectedShot.minuteStartSec) : t(locale, 'controlPreviewHint')}
            </p>
          </div>
          {selectedShot && assetAccess ? (
            previewBroken ? (
              <p className="rounded-lg border border-[var(--error-border)] bg-[var(--error-bg)] px-3 py-4 text-sm text-[var(--error-text)]">
                {t(locale, 'controlScreenshotError')}
              </p>
            ) : (
              <div className="rounded-lg bg-[var(--surface-muted)] p-2">
                <img
                  src={`${getShotUrl(selectedShot.name, assetAccess)}${assetAccess.accessToken ? '&' : '?'}t=${selectedShot.minuteStartSec}`}
                  alt={selectedShot.name}
                  className="max-h-[32rem] w-full rounded-lg bg-[var(--surface-muted)] object-contain"
                  onError={() => setPreviewBroken(true)}
                />
              </div>
            )
          ) : (
            <div className="rounded-lg border border-dashed border-[var(--card-border)] bg-[var(--surface-muted)] px-3 py-10 text-center text-sm text-[var(--text-secondary)]">
              {t(locale, 'controlPreviewEmpty')}
            </div>
          )}
          {selectedShot && (
            <p className="mt-2 break-all text-[11px] text-[var(--text-tertiary)]">{selectedShot.name}</p>
          )}
        </section>
      </div>
    </section>
  );
}
