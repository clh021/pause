import { useCallback, useEffect, useRef, useState } from 'react';
import {
  forceBreak,
  forceUnlock,
  getActivity,
  getAutoScreenshot,
  getShotUrl,
  getScreenshotUrl,
  setAutoScreenshot,
  takeScreenshot
} from '../api';
import { t, type Locale } from '../i18n';
import type { ActivitySummary, RuntimeState, ShotInfo } from '../types';

type ControlPageProps = {
  locale: Locale;
  runtime: RuntimeState | null;
  onRuntimeRefresh: () => void;
};

type TimeRange = '1h' | '6h' | '24h';
const RANGE_SEC: Record<TimeRange, number> = { '1h': 3600, '6h': 21600, '24h': 86400 };
const BAR_COUNT = 120; // show 120 bars in the mini-chart

export function ControlPage({ locale, runtime, onRuntimeRefresh }: ControlPageProps) {
  const [activity, setActivity] = useState<ActivitySummary | null>(null);
  const [selectedRange, setSelectedRange] = useState<TimeRange>('24h');
  const [autoShot, setAutoShot] = useState(false);
  const [selectedShot, setSelectedShot] = useState<ShotInfo | null>(null);
  const [loading, setLoading] = useState(true);
  const [actionLoading, setActionLoading] = useState<string | null>(null);
  const [actionMsg, setActionMsg] = useState<{ key: string; text: string; ok: boolean } | null>(null);
  const [manScreenshotUrl, setManScreenshotUrl] = useState<string | null>(null);
  const msgTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const pollTimer = useRef<ReturnType<typeof setInterval> | null>(null);

  const showMsg = useCallback((key: string, text: string, ok: boolean) => {
    setActionMsg({ key, text, ok });
    if (msgTimer.current) clearTimeout(msgTimer.current);
    msgTimer.current = setTimeout(() => setActionMsg(null), 4000);
  }, []);

  const fetchActivity = useCallback(async (range: TimeRange) => {
    setLoading(true);
    try {
      const to = Math.floor(Date.now() / 1000);
      const from = to - RANGE_SEC[range];
      const data = await getActivity(from, to);
      setActivity(data);
    } catch { /* ignore */ } finally {
      setLoading(false);
    }
  }, []);

  // Load initial data
  useEffect(() => {
    const init = async () => {
      await fetchActivity(selectedRange);
      try { setAutoShot(await getAutoScreenshot()); } catch { /* ignore */ }
    };
    void init();
  }, [fetchActivity, selectedRange]);

  // Poll activity every 10s
  useEffect(() => {
    pollTimer.current = setInterval(() => {
      void fetchActivity(selectedRange);
    }, 10000);
    return () => {
      if (pollTimer.current) clearInterval(pollTimer.current);
    };
  }, [fetchActivity, selectedRange]);

  const handleRangeChange = useCallback((range: TimeRange) => {
    setSelectedRange(range);
    setSelectedShot(null);
  }, []);

  const handleManualScreenshot = useCallback(async () => {
    setActionLoading('manualShot');
    try {
      await takeScreenshot();
      setManScreenshotUrl(`${getScreenshotUrl()}?t=${Date.now()}`);
      showMsg('manualShot', t(locale, 'controlShotDone'), true);
      void fetchActivity(selectedRange);
    } catch (err) {
      showMsg('manualShot', String(err), false);
    } finally {
      setActionLoading(null);
    }
  }, [locale, showMsg, fetchActivity, selectedRange]);

  const handleForceBreak = useCallback(async () => {
    setActionLoading('forceBreak');
    try {
      await forceBreak();
      showMsg('forceBreak', t(locale, 'controlBreakDone'), true);
      onRuntimeRefresh();
    } catch (err) {
      showMsg('forceBreak', String(err), false);
    } finally {
      setActionLoading(null);
    }
  }, [locale, showMsg, onRuntimeRefresh]);

  const handleForceUnlock = useCallback(async () => {
    setActionLoading('forceUnlock');
    try {
      await forceUnlock();
      showMsg('forceUnlock', t(locale, 'controlUnlockDone'), true);
      onRuntimeRefresh();
    } catch (err) {
      showMsg('forceUnlock', String(err), false);
    } finally {
      setActionLoading(null);
    }
  }, [locale, showMsg, onRuntimeRefresh]);

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

  // Preview a screenshot
  const handleShotClick = useCallback((shot: ShotInfo) => {
    setSelectedShot(shot);
  }, []);

  const isResting = runtime?.currentSession?.status === 'resting';

  // ---- Build mini timeline ----
  const activityBars: { active: boolean; ts: number }[] = [];
  if (activity && activity.ticks.length > 0) {
    const step = Math.max(1, Math.floor(activity.ticks.length / BAR_COUNT));
    for (let i = 0; i < activity.ticks.length && activityBars.length < BAR_COUNT; i += step) {
      activityBars.push({ active: activity.ticks[i].a, ts: activity.ticks[i].t });
    }
  }

  // Map shots to timeline positions
  const rangeSec = RANGE_SEC[selectedRange];
  const nowTs = Math.floor(Date.now() / 1000);
  const fromTs = nowTs - rangeSec;

  // ---- Summarize ----
  const activeMin = activity ? Math.round(activity.activeSec / 60) : 0;
  const idleMin = activity ? Math.round(activity.idleSec / 60) : 0;

  const btnBase =
    'inline-flex cursor-pointer items-center justify-center gap-2 rounded-lg border-0 px-5 py-3 text-sm font-medium transition-all duration-150 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--control-focus-ring)] disabled:opacity-40';
  const btnPrimary = `${btnBase} bg-[var(--accent-bg)] text-white hover:brightness-110`;
  const btnDanger = `${btnBase} bg-[#e81123] text-white hover:bg-[#c50f1f]`;
  const btnLoading = 'cursor-wait';

  return (
    <section className="mt-3 space-y-5 px-2 pb-4 sm:px-3">
      {/* Status bar */}
      <div className="flex items-center justify-between gap-3 rounded-xl border border-[var(--card-border)] bg-[var(--card-bg)] px-4 py-3 shadow-[var(--shadow-subtle)]">
        <div className="flex items-center gap-3">
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
        {actionMsg && (
          <span className={`text-xs ${actionMsg.ok ? 'text-[#22c55e]' : 'text-[#e81123]'}`}>
            {actionMsg.text}
          </span>
        )}
      </div>

      {/* Quick actions row */}
      <div className="flex flex-wrap gap-3">
        <button
          type="button"
          className={`${btnPrimary} ${actionLoading === 'forceBreak' ? btnLoading : ''}`}
          disabled={actionLoading !== null}
          onClick={() => void handleForceBreak()}
        >
          {t(locale, 'controlForceBreak')}
        </button>
        <button
          type="button"
          className={`${btnDanger} ${actionLoading === 'forceUnlock' ? btnLoading : ''}`}
          disabled={actionLoading !== null || !isResting}
          onClick={() => void handleForceUnlock()}
        >
          {t(locale, 'controlForceUnlock')}
        </button>
        <button
          type="button"
          className={`${btnPrimary} ${actionLoading === 'manualShot' ? btnLoading : ''}`}
          disabled={actionLoading !== null}
          onClick={() => void handleManualScreenshot()}
        >
          {t(locale, 'controlManualShot')}
        </button>
      </div>

      {/* Auto screenshot toggle */}
      <div className="flex items-center gap-3 rounded-xl border border-[var(--card-border)] bg-[var(--card-bg)] px-4 py-3 shadow-[var(--shadow-subtle)]">
        <label className="flex cursor-pointer items-center gap-3">
          <input
            type="checkbox"
            className="h-4 w-4 accent-[var(--accent-bg)]"
            checked={autoShot}
            onChange={() => void handleAutoShotToggle()}
          />
          <span className="text-sm text-[var(--text-primary)]">{t(locale, 'controlAutoShot')}</span>
        </label>
      </div>

      {/* Manual screenshot preview */}
      {manScreenshotUrl && (
        <div className="rounded-xl border border-[var(--card-border)] bg-[var(--card-bg)] p-4 shadow-[var(--shadow-subtle)]">
          <h3 className="mb-2 text-sm font-semibold text-[var(--text-primary)]">{t(locale, 'controlManualShot')}</h3>
          <img src={manScreenshotUrl} alt="screenshot" className="max-h-60 w-full rounded-lg object-contain" />
        </div>
      )}

      {/* Activity timeline */}
      <div className="rounded-xl border border-[var(--card-border)] bg-[var(--card-bg)] p-4 shadow-[var(--shadow-subtle)]">
        <div className="mb-3 flex items-center justify-between">
          <h2 className="text-sm font-semibold text-[var(--text-primary)]">
            {t(locale, 'controlTimeline')}
            {activity && (
              <span className="ml-2 text-xs font-normal text-[var(--text-secondary)]">
                {activeMin}m {t(locale, 'controlActive')} / {idleMin}m {t(locale, 'controlIdle')}
              </span>
            )}
          </h2>
          <div className="flex gap-1">
            {(['1h', '6h', '24h'] as TimeRange[]).map((r) => (
              <button
                key={r}
                type="button"
                className={`cursor-pointer rounded-md border-0 px-3 py-1 text-xs font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--control-focus-ring)] ${
                  selectedRange === r
                    ? 'bg-[var(--accent-bg)] text-white'
                    : 'bg-[var(--seg-bg)] text-[var(--text-secondary)] hover:bg-[var(--seg-hover-bg)]'
                }`}
                onClick={() => handleRangeChange(r)}
              >
                {r}
              </button>
            ))}
          </div>
        </div>

        {loading && activityBars.length === 0 ? (
          <p className="py-4 text-center text-xs text-[var(--text-secondary)]">{t(locale, 'loading')}</p>
        ) : activityBars.length === 0 ? (
          <p className="py-4 text-center text-xs text-[var(--text-secondary)]">{t(locale, 'controlTimelineEmpty')}</p>
        ) : (
          <>
            {/* Bar chart */}
            <div className="flex h-12 items-end gap-[2px] overflow-x-auto pb-1">
              {activityBars.map((bar, i) => (
                <div
                  key={i}
                  className={`shrink-0 rounded-t-sm ${
                    bar.active ? 'bg-[#22c55e]' : 'bg-[#374151]'
                  }`}
                  style={{ width: `${100 / Math.max(activityBars.length, 1)}%`, height: bar.active ? '100%' : '30%' }}
                  title={new Date(bar.ts * 1000).toLocaleTimeString()}
                />
              ))}
            </div>

            {/* Screenshot thumbnails on timeline */}
            {activity!.shots.length > 0 && activity!.shots.map((shot) => (
                    <button
                      key={shot.name}
                      type="button"
                      className={`group relative cursor-pointer overflow-hidden rounded-lg border ${
                        selectedShot?.name === shot.name
                          ? 'border-[var(--accent-bg)] ring-2 ring-[var(--accent-bg)]'
                          : 'border-[var(--card-border)]'
                      }`}
                      onClick={() => handleShotClick(shot)}
                    >
                      <img
                        src={`${getShotUrl(shot.name)}?t=${shot.t}`}
                        alt={shot.name}
                        className="h-16 w-24 object-cover transition-opacity group-hover:opacity-80"
                      />
                      <div className="absolute bottom-0 left-0 right-0 bg-black/50 px-1 py-0.5 text-[10px] text-white">
                        {new Date(shot.t * 1000).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })}
                      </div>
                    </button>
                  ))
                }
          </>
        )}
      </div>

      {/* Full screenshot preview */}
      {selectedShot && (
        <div className="rounded-xl border border-[var(--card-border)] bg-[var(--card-bg)] p-4 shadow-[var(--shadow-subtle)]">
          <div className="mb-2 flex items-center justify-between">
            <h3 className="text-sm font-semibold text-[var(--text-primary)]">
              {new Date(selectedShot.t * 1000).toLocaleString()}
            </h3>
            <button
              type="button"
              className="cursor-pointer rounded-md border-0 bg-transparent px-2 py-1 text-xs text-[var(--text-secondary)] hover:bg-[var(--seg-hover-bg)]"
              onClick={() => setSelectedShot(null)}
            >
              {t(locale, 'close')}
            </button>
          </div>
          <img
            src={`${getShotUrl(selectedShot.name)}?t=${selectedShot.t}`}
            alt={selectedShot.name}
            className="max-h-[70vh] w-full rounded-lg object-contain"
          />
        </div>
      )}
    </section>
  );
}
