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

type HourDisplayBlock =
  | {
      kind: 'hour';
      hourStartSec: number;
      minutes: ActivityMinute[];
      activeMinutes: number;
      screenshotCount: number;
    }
  | {
      kind: 'empty-range';
      startHourSec: number;
      endHourSec: number;
      hourCount: number;
    };

function chunkByHour(minutes: ActivityMinute[]): ActivityMinute[][] {
  const hours: ActivityMinute[][] = [];
  for (let i = 0; i < minutes.length; i += 60) {
    hours.push(minutes.slice(i, i + 60));
  }
  return hours;
}

function countActiveMinutes(minutes: ActivityMinute[]): number {
  let count = 0;
  for (const minute of minutes) {
    if (minute.active) {
      count++;
    }
  }
  return count;
}

function countScreenshots(minutes: ActivityMinute[]): number {
  let count = 0;
  for (const minute of minutes) {
    if (minute.hasScreenshot) {
      count++;
    }
  }
  return count;
}

function isEmptyHour(minutes: ActivityMinute[]): boolean {
  for (const minute of minutes) {
    if (minute.active || minute.hasScreenshot) {
      return false;
    }
  }
  return true;
}

function buildHourDisplayBlocks(groupedHours: ActivityMinute[][], showEmptyHours: boolean): HourDisplayBlock[] {
  const blocks: HourDisplayBlock[] = [];
  let emptyRangeStart = -1;

  const flushEmptyRange = (endExclusive: number) => {
    if (emptyRangeStart < 0) {
      return;
    }
    const hourCount = endExclusive - emptyRangeStart;
    if (hourCount >= 2 && !showEmptyHours) {
      const startHour = groupedHours[emptyRangeStart]?.[0]?.minuteStartSec ?? 0;
      const endHour = groupedHours[endExclusive - 1]?.[0]?.minuteStartSec ?? startHour;
      blocks.push({
        kind: 'empty-range',
        startHourSec: startHour,
        endHourSec: endHour,
        hourCount
      });
    } else {
      for (let i = emptyRangeStart; i < endExclusive; i++) {
        const minutes = groupedHours[i] ?? [];
        const hourStartSec = minutes[0]?.minuteStartSec ?? 0;
        blocks.push({
          kind: 'hour',
          hourStartSec,
          minutes,
          activeMinutes: 0,
          screenshotCount: 0
        });
      }
    }
    emptyRangeStart = -1;
  };

  for (let i = 0; i < groupedHours.length; i++) {
    const minutes = groupedHours[i] ?? [];
    if (isEmptyHour(minutes)) {
      if (emptyRangeStart < 0) {
        emptyRangeStart = i;
      }
      continue;
    }
    flushEmptyRange(i);
    blocks.push({
      kind: 'hour',
      hourStartSec: minutes[0]?.minuteStartSec ?? 0,
      minutes,
      activeMinutes: countActiveMinutes(minutes),
      screenshotCount: countScreenshots(minutes)
    });
  }
  flushEmptyRange(groupedHours.length);

  return blocks;
}

function formatHourLabel(locale: Locale, minuteStartSec: number): string {
  return new Date(minuteStartSec * 1000).toLocaleString(locale, {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit'
  });
}

function formatHourRangeLabel(locale: Locale, startHourSec: number, endHourSec: number): string {
  const start = new Date(startHourSec * 1000);
  const end = new Date((endHourSec + 59 * 60) * 1000);
  if (locale === 'zh-CN') {
    return `${start.toLocaleString(locale, {
      month: '2-digit',
      day: '2-digit',
      hour: '2-digit'
    })} - ${end.toLocaleString(locale, {
      month: '2-digit',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit'
    })}`;
  }
  return `${start.toLocaleString(locale, {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit'
  })} - ${end.toLocaleString(locale, {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit'
  })}`;
}

function formatCollapsedEmptyHoursText(locale: Locale, hourCount: number): string {
  return locale === 'zh-CN' ? `已折叠连续 ${hourCount} 个空小时` : `${hourCount} consecutive empty hours collapsed`;
}

function formatMinuteLabel(locale: Locale, minuteStartSec: number): string {
  return new Date(minuteStartSec * 1000).toLocaleString(locale, {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit'
  });
}

function formatMinuteNumber(minuteStartSec: number): string {
  return String(new Date(minuteStartSec * 1000).getMinutes()).padStart(2, '0');
}

function normalizePositiveInteger(value: string): string {
  return value.replace(/\D+/g, '');
}

export function ControlPage({ locale, runtime, onRuntimeRefresh }: ControlPageProps) {
  const [activity, setActivity] = useState<ActivitySummary | null>(null);
  const [autoShot, setAutoShot] = useState(false);
  const [selectedShot, setSelectedShot] = useState<PreviewState | null>(null);
  const [showEmptyHours, setShowEmptyHours] = useState(false);
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

  useEffect(() => {
    if (!selectedShot) {
      return;
    }
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        setPreviewBroken(false);
        setSelectedShot(null);
      }
    };
    window.addEventListener('keydown', onKeyDown);
    return () => {
      window.removeEventListener('keydown', onKeyDown);
    };
  }, [selectedShot]);

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
  const collapsibleEmptyHourCount = useMemo(() => {
    let total = 0;
    let streak = 0;
    for (const hour of groupedHours) {
      if (isEmptyHour(hour)) {
        streak++;
        continue;
      }
      if (streak >= 2) {
        total += streak;
      }
      streak = 0;
    }
    if (streak >= 2) {
      total += streak;
    }
    return total;
  }, [groupedHours]);
  const hiddenEmptyHourCount = showEmptyHours ? 0 : collapsibleEmptyHourCount;
  const displayBlocks = useMemo(() => {
    const blocks = buildHourDisplayBlocks(groupedHours, showEmptyHours);
    return [...blocks].reverse();
  }, [groupedHours, showEmptyHours]);

  const isResting = runtime?.currentSession?.status === 'resting';
  const btnBase =
    'inline-flex cursor-pointer items-center justify-center gap-2 rounded-lg border border-transparent px-4 py-2.5 text-sm font-medium transition-colors duration-150 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--control-focus-ring)] disabled:cursor-not-allowed disabled:opacity-45';
  const btnPrimary = `${btnBase} bg-[var(--accent-bg)] text-white hover:brightness-110`;
  const btnDanger = `${btnBase} bg-[var(--danger-bg)] text-white hover:bg-[var(--danger-bg-hover)]`;
  const btnGhost = `${btnBase} border-[var(--surface-border)] bg-[var(--app-bg)] text-[var(--text-primary)] hover:bg-[var(--seg-hover-bg)]`;

  return (
    <section className="mt-3 space-y-5 px-2 pb-4 sm:px-3">
      <div className="flex items-center justify-between gap-3 rounded-xl border border-[var(--surface-border)] bg-[var(--app-bg)] px-4 py-3 shadow-[var(--shadow-subtle)]">
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
        <div className="rounded-xl border border-[var(--surface-border)] bg-[var(--app-bg)] p-4 shadow-[var(--shadow-subtle)]">
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
            <label className="flex cursor-pointer items-center gap-3 rounded-lg border border-[var(--surface-border)] bg-[var(--surface-bg)] px-3 py-2">
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

        <div className="rounded-xl border border-[var(--surface-border)] bg-[var(--app-bg)] p-4 shadow-[var(--shadow-subtle)]">
          <div className="mb-4 flex flex-wrap items-start justify-between gap-3">
            <div>
              <h2 className="text-sm font-semibold text-[var(--text-primary)]">{t(locale, 'controlTimeline')}</h2>
              <p className="mt-1 text-xs text-[var(--text-secondary)]">
                {activity
                  ? `${activity.activeMinutes}m ${t(locale, 'controlActive')} / ${activity.idleMinutes}m ${t(locale, 'controlIdle')}`
                  : t(locale, 'loading')}
              </p>
            </div>
            <div className="flex flex-wrap items-center justify-end gap-3 text-xs text-[var(--text-secondary)]">
              {collapsibleEmptyHourCount > 0 && (
                <button
                  type="button"
                  className="inline-flex cursor-pointer items-center gap-2 rounded-md border border-[var(--surface-border)] bg-[var(--surface-bg)] px-2.5 py-1.5 text-xs text-[var(--text-primary)] transition-colors hover:bg-[var(--seg-hover-bg)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--control-focus-ring)]"
                  onClick={() => setShowEmptyHours((prev) => !prev)}
                >
                  {showEmptyHours ? t(locale, 'controlHideEmptyHours') : `${t(locale, 'controlShowEmptyHours')} (${hiddenEmptyHourCount})`}
                </button>
              )}
              <span className="inline-flex items-center gap-2">
                <span className="h-3 w-3 rounded-full border border-[var(--control-dot-idle-border)] bg-transparent" />
                {t(locale, 'controlLegendIdle')}
              </span>
              <span className="inline-flex items-center gap-2">
                <span className="h-3 w-3 rounded-full border border-[var(--control-dot-active)] bg-[var(--control-dot-active)]" />
                {t(locale, 'controlLegendActive')}
              </span>
              <span className="inline-flex items-center gap-2">
                <span className="h-3 w-3 rounded-full border border-[var(--control-dot-shot-border)] bg-[var(--control-dot-active)] ring-1 ring-[var(--control-dot-shot-border)] ring-offset-1 ring-offset-[var(--app-bg)]" />
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
              {displayBlocks.map((block) => {
                if (block.kind === 'empty-range') {
                  return (
                    <article
                      key={`empty-${block.startHourSec}-${block.endHourSec}`}
                      className="rounded-lg border border-dashed border-[var(--surface-border)] bg-[var(--surface-bg)] px-4 py-3 shadow-[var(--shadow-soft)]"
                    >
                      <div className="flex flex-wrap items-center justify-between gap-2">
                        <div>
                          <p className="text-sm font-semibold text-[var(--text-primary)]">
                            {formatHourRangeLabel(locale, block.startHourSec, block.endHourSec)}
                          </p>
                          <p className="mt-1 text-xs text-[var(--text-secondary)]">{formatCollapsedEmptyHoursText(locale, block.hourCount)}</p>
                        </div>
                        {!showEmptyHours && (
                          <button
                            type="button"
                            className="inline-flex cursor-pointer items-center rounded-md border border-[var(--surface-border)] bg-[var(--app-bg)] px-2.5 py-1.5 text-xs text-[var(--text-primary)] transition-colors hover:bg-[var(--seg-hover-bg)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--control-focus-ring)]"
                            onClick={() => setShowEmptyHours(true)}
                          >
                            {t(locale, 'controlShowEmptyHours')}
                          </button>
                        )}
                      </div>
                    </article>
                  );
                }

                // Infer latest hour from server data to avoid client/server clock skew
                const minutesArr = activity?.minutes ?? [];
                const latestMinute = minutesArr.length > 0
                  ? minutesArr[minutesArr.length - 1].minuteStartSec
                  : Math.floor(Date.now() / 1000);
                const currentHourStart = latestMinute - (latestMinute % 3600);
                const currentMinuteStart = latestMinute; // minuteStartSec is already minute-aligned
                const isCurrentHour = block.hourStartSec === currentHourStart;
                const visibleMinutes = isCurrentHour
                  ? block.minutes.filter(m => m.minuteStartSec < currentMinuteStart)
                  : block.minutes;
                const displayActiveMinutes = isCurrentHour ? countActiveMinutes(visibleMinutes) : block.activeMinutes;
                const displayScreenshotCount = isCurrentHour ? countScreenshots(visibleMinutes) : block.screenshotCount;

                if (isCurrentHour && visibleMinutes.length === 0) {
                  // Show a placeholder card when the current hour has no completed minute yet
                  return (
                    <article
                      key={block.hourStartSec}
                      className="rounded-lg border border-[var(--surface-border)] bg-[var(--surface-bg)] p-4 shadow-[var(--shadow-soft)]"
                    >
                      <header className="mb-3 flex flex-wrap items-center justify-between gap-2">
                        <span className="text-xs font-semibold text-[var(--text-primary)]">{formatHourLabel(locale, block.hourStartSec)}</span>
                        <div className="flex flex-wrap items-center gap-2 text-[11px] text-[var(--text-tertiary)]">
                          <span>{t(locale, 'controlHourLabel')}</span>
                        </div>
                      </header>
                      <p className="py-3 text-center text-xs text-[var(--text-tertiary)]">{t(locale, 'controlAwaitingData')}</p>
                    </article>
                  );
                }

                return (
                  <article
                    key={block.hourStartSec}
                    className="rounded-lg border border-[var(--surface-border)] bg-[var(--surface-bg)] p-4 shadow-[var(--shadow-soft)]"
                  >
                    <header className="mb-3 flex flex-wrap items-center justify-between gap-2">
                      <span className="text-xs font-semibold text-[var(--text-primary)]">{formatHourLabel(locale, block.hourStartSec)}</span>
                      <div className="flex flex-wrap items-center gap-2 text-[11px] text-[var(--text-tertiary)]">
                        <span>{t(locale, 'controlHourLabel')}</span>
                        <span>{`${displayActiveMinutes}m ${t(locale, 'controlActive')}`}</span>
                        <span>{`${displayScreenshotCount} ${t(locale, 'controlShots')}`}</span>
                      </div>
                    </header>
                    <div className="grid grid-cols-[repeat(12,minmax(0,1fr))] justify-items-center gap-1.5 md:grid-cols-[repeat(20,minmax(0,1fr))]">
                      {visibleMinutes.map((minute) => {
                        const isSelected = selectedShot?.name === minute.shotName;
                        const className = minute.hasScreenshot
                          ? `border-[var(--control-dot-shot-border)] bg-[var(--control-dot-active)] ring-1 ring-[var(--control-dot-shot-border)] ring-offset-1 ring-offset-[var(--surface-bg)] ${
                              isSelected ? 'scale-[1.02] shadow-[0_0_0_1px_var(--control-dot-shot-border)]' : ''
                            }`
                          : minute.active
                            ? 'border-[var(--control-dot-active)] bg-[var(--control-dot-active)]'
                            : 'border-[var(--control-dot-idle-border)] bg-transparent';
                        const textClassName = minute.hasScreenshot || minute.active
                          ? 'text-[var(--control-dot-text-strong)]'
                          : 'text-[var(--control-dot-text-muted)]';
                        return (
                          <button
                            key={minute.minuteStartSec}
                            type="button"
                            disabled={!minute.hasScreenshot}
                            className={`flex h-6 w-6 items-center justify-center rounded-full border text-[10px] font-semibold leading-none transition-transform duration-150 md:h-6 md:w-6 md:text-[10px] ${
                              minute.hasScreenshot ? 'cursor-pointer hover:scale-105' : 'cursor-default'
                            } ${className} ${textClassName}`}
                            title={formatMinuteLabel(locale, minute.minuteStartSec)}
                            onClick={() => {
                              if (!minute.shotName) return;
                              setPreviewBroken(false);
                              setSelectedShot({ name: minute.shotName, minuteStartSec: minute.minuteStartSec });
                            }}
                          >
                            {formatMinuteNumber(minute.minuteStartSec)}
                          </button>
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
          <section className="rounded-xl border border-[var(--surface-border)] bg-[var(--app-bg)] p-4 shadow-[var(--shadow-subtle)]">
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
            <div className="rounded-lg bg-[var(--surface-bg)] p-2">
              <img
                src={manualPreviewUrl}
                alt="manual screenshot"
                className="max-h-[22rem] w-full rounded-lg bg-[var(--surface-bg)] object-contain"
              />
            </div>
          </section>
        )}

      </div>

      {selectedShot && (
        <>
          <button
            type="button"
            aria-label={t(locale, 'close')}
            className="fixed inset-[-8px] z-30 bg-[var(--dialog-scrim)]"
            onClick={() => {
              setPreviewBroken(false);
              setSelectedShot(null);
            }}
          />
          <section
            role="dialog"
            aria-modal="true"
            aria-label={t(locale, 'controlScreenshot')}
            className="fixed inset-x-3 top-1/2 z-40 max-h-[calc(100vh-2rem)] -translate-y-1/2 rounded-[16px] border border-[var(--surface-border-strong)] bg-[var(--surface-bg)] p-4 shadow-[var(--surface-shadow)] sm:left-1/2 sm:right-auto sm:w-[min(72rem,calc(100vw-2rem))] sm:-translate-x-1/2"
            onClick={(event) => event.stopPropagation()}
          >
            <div className="mb-3 flex items-start justify-between gap-3">
              <div className="min-w-0">
                <h3 className="text-sm font-semibold text-[var(--text-primary)]">{t(locale, 'controlScreenshot')}</h3>
                <p className="mt-1 text-xs text-[var(--text-secondary)]">{formatMinuteLabel(locale, selectedShot.minuteStartSec)}</p>
                <p className="mt-1 break-all text-[11px] text-[var(--text-tertiary)]">{selectedShot.name}</p>
              </div>
              <button
                type="button"
                className="inline-flex cursor-pointer items-center rounded-md border border-[var(--surface-border)] bg-[var(--app-bg)] px-2.5 py-1.5 text-xs text-[var(--text-primary)] transition-colors hover:bg-[var(--seg-hover-bg)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--control-focus-ring)]"
                onClick={() => {
                  setPreviewBroken(false);
                  setSelectedShot(null);
                }}
              >
                {t(locale, 'close')}
              </button>
            </div>
            {assetAccess ? (
              previewBroken ? (
                <p className="rounded-lg border border-[var(--error-border)] bg-[var(--error-bg)] px-3 py-4 text-sm text-[var(--error-text)]">
                  {t(locale, 'controlScreenshotError')}
                </p>
              ) : (
                <div className="rounded-lg bg-[var(--app-bg)] p-2">
                  <img
                    src={`${getShotUrl(selectedShot.name, assetAccess)}${assetAccess.accessToken ? '&' : '?'}t=${selectedShot.minuteStartSec}`}
                    alt={selectedShot.name}
                    className="max-h-[calc(100vh-10rem)] w-full rounded-lg bg-[var(--app-bg)] object-contain"
                    onError={() => setPreviewBroken(true)}
                  />
                </div>
              )
            ) : (
              <div className="rounded-lg border border-dashed border-[var(--surface-border)] bg-[var(--app-bg)] px-3 py-10 text-center text-sm text-[var(--text-secondary)]">
                {t(locale, 'controlPreviewEmpty')}
              </div>
            )}
          </section>
        </>
      )}
    </section>
  );
}
