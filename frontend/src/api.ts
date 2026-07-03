import type {
  AnalyticsBreakTypeDistribution,
  AnalyticsSummary,
  AnalyticsTrend,
  AnalyticsWeeklyStats,
  NotificationCapability,
  ReminderConfig,
  ReminderCreateInput,
  ReminderPatch,
  ShotInfo,
  RuntimeState,
  Settings,
  SettingsPatch,
  PlatformInfo,
  RemoteServerInfo,
  UpdateAsset,
  UpdateCheckResult,
  ActivitySummary
} from './types';

function isWebMode(): boolean {
  return !(window as unknown as { go?: { app?: { App?: unknown } } }).go?.app?.App;
}

export const ERR_REMOTE_AUTH_REQUIRED = 'ERR_REMOTE_AUTH_REQUIRED';
export const ERR_REMOTE_AUTH_INVALID = 'ERR_REMOTE_AUTH_INVALID';
export const ERR_REMOTE_CONTROL_UNAVAILABLE = 'ERR_REMOTE_CONTROL_UNAVAILABLE';

export type RemoteAssetAccess = {
  baseUrl: string;
  accessToken: string;
};

let nativeRemoteServerInfoPromise: Promise<RemoteServerInfo> | null = null;

async function remoteFetch(path: string, options: RequestInit = {}): Promise<Response> {
  const context = await getRemoteRequestContext();
  const headers = new Headers(options.headers || {});
  if (!headers.has('Content-Type')) {
    headers.set('Content-Type', 'application/json');
  }
  if (context.accessToken !== '') {
    headers.set('Authorization', `Bearer ${context.accessToken}`);
  }
  const url = `${context.baseUrl}${path}`;
  const res = await fetch(url, {
    ...options,
    credentials: 'same-origin',
    headers
  });
  if (res.status === 401) {
    throw new Error(isWebMode() ? ERR_REMOTE_AUTH_REQUIRED : ERR_REMOTE_AUTH_INVALID);
  }
  if (!res.ok) {
    const body = await res.json().catch(() => ({ error: res.statusText }));
    throw new Error(body.error || `HTTP ${res.status}`);
  }
  return res;
}

type Backend = {
  GetSettings: () => Promise<Settings>;
  UpdateSettings: (patch: SettingsPatch) => Promise<Settings>;
  GetReminders: () => Promise<ReminderConfig[]>;
  CreateReminder: (input: ReminderCreateInput) => Promise<ReminderConfig[]>;
  DeleteReminder: (reminderID: number) => Promise<ReminderConfig[]>;
  UpdateReminder: (patch: ReminderPatch) => Promise<ReminderConfig[]>;
  GetLaunchAtLogin: () => Promise<boolean>;
  SetLaunchAtLogin: (enabled: boolean) => Promise<boolean>;
  GetRuntimeState: () => Promise<RuntimeState>;
  GetAnalyticsWeeklyStats: (fromSec: number, toSec: number) => Promise<AnalyticsWeeklyStats>;
  GetAnalyticsSummary: (fromSec: number, toSec: number) => Promise<AnalyticsSummary>;
  GetAnalyticsTrendByDay: (fromSec: number, toSec: number) => Promise<AnalyticsTrend>;
  GetAnalyticsBreakTypeDistribution: (fromSec: number, toSec: number) => Promise<AnalyticsBreakTypeDistribution>;
  Pause: () => Promise<RuntimeState>;
  Resume: () => Promise<RuntimeState>;
  SkipCurrentBreak: () => Promise<RuntimeState>;
  PostponeCurrentBreak: () => Promise<RuntimeState>;
  GetNotificationCapability: () => Promise<NotificationCapability>;
  RequestNotificationPermission: () => Promise<NotificationCapability>;
  OpenNotificationSettings: () => Promise<void>;
  GetPlatformInfo?: () => Promise<PlatformInfo>;
  GetRemoteServerInfo?: () => Promise<RemoteServerInfo>;
  Quit?: () => Promise<void> | void;
  CloseWindow?: () => Promise<void> | void;
  StartBreakNow?: () => Promise<RuntimeState>;
};

function getBackend(): Backend | null {
  const maybe = (window as unknown as { go?: { app?: { App?: Backend } } }).go;
  return maybe?.app?.App ?? null;
}

function missingBackendError(): Error {
  return new Error('Pause backend bridge unavailable (window.go.app.App is missing).');
}

function requireBackend(): Backend {
  const backend = getBackend();
  if (!backend) {
    throw missingBackendError();
  }
  return backend;
}

async function getNativeRemoteServerInfo(): Promise<RemoteServerInfo> {
  if (isWebMode()) {
    return {
      enabled: true,
      running: true,
      localBaseUrl: '',
      tokenRequired: true
    };
  }
  if (nativeRemoteServerInfoPromise) {
    return nativeRemoteServerInfoPromise;
  }
  const backend = requireBackend();
  if (!backend.GetRemoteServerInfo) {
    throw new Error(ERR_REMOTE_CONTROL_UNAVAILABLE);
  }
  nativeRemoteServerInfoPromise = backend.GetRemoteServerInfo().catch((error) => {
    nativeRemoteServerInfoPromise = null;
    throw error;
  });
  return nativeRemoteServerInfoPromise;
}

async function getRemoteRequestContext(): Promise<RemoteAssetAccess> {
  if (isWebMode()) {
    return { baseUrl: '', accessToken: '' };
  }
  const info = await getNativeRemoteServerInfo();
  if (!info.enabled || !info.running || info.localBaseUrl.trim() === '') {
    throw new Error(info.lastError || ERR_REMOTE_CONTROL_UNAVAILABLE);
  }
  return {
    baseUrl: info.localBaseUrl,
    accessToken: info.accessToken?.trim() || ''
  };
}

function normalizeReminderConfigs(payload: ReminderConfig[] | null | undefined): ReminderConfig[] {
  return Array.isArray(payload) ? payload : [];
}

export async function getSettings(): Promise<Settings> {
  if (isWebMode()) {
    const res = await remoteFetch('/api/settings');
    return res.json();
  }
  return requireBackend().GetSettings();
}

export async function updateSettings(patch: SettingsPatch): Promise<Settings> {
  if (isWebMode()) {
    const res = await remoteFetch('/api/settings/update', {
      method: 'PATCH',
      body: JSON.stringify(patch)
    });
    return res.json();
  }
  return requireBackend().UpdateSettings(patch);
}

export async function getReminders(): Promise<ReminderConfig[]> {
  if (isWebMode()) {
    const res = await remoteFetch('/api/reminders');
    return res.json();
  }
  return normalizeReminderConfigs(await requireBackend().GetReminders());
}

export async function createReminder(input: ReminderCreateInput): Promise<ReminderConfig[]> {
  if (isWebMode()) {
    const res = await remoteFetch('/api/reminders/create', {
      method: 'POST',
      body: JSON.stringify(input)
    });
    return res.json();
  }
  return normalizeReminderConfigs(await requireBackend().CreateReminder(input));
}

export async function deleteReminder(reminderID: number): Promise<ReminderConfig[]> {
  if (isWebMode()) {
    const res = await remoteFetch(`/api/reminders/delete/${reminderID}`, {
      method: 'DELETE'
    });
    return res.json();
  }
  return normalizeReminderConfigs(await requireBackend().DeleteReminder(reminderID));
}

export async function updateReminder(patch: ReminderPatch): Promise<ReminderConfig[]> {
  if (isWebMode()) {
    const res = await remoteFetch(`/api/reminders/update/${patch.id}`, {
      method: 'PUT',
      body: JSON.stringify(patch)
    });
    return res.json();
  }
  return normalizeReminderConfigs(await requireBackend().UpdateReminder(patch));
}

export async function getLaunchAtLogin(): Promise<boolean> {
  if (isWebMode()) {
    const res = await remoteFetch('/api/settings/launch-at-login');
    const data = (await res.json()) as { enabled: boolean };
    return data.enabled;
  }
  return requireBackend().GetLaunchAtLogin();
}

export async function setLaunchAtLogin(enabled: boolean): Promise<boolean> {
  if (isWebMode()) {
    const res = await remoteFetch('/api/settings/launch-at-login/set', {
      method: 'POST',
      body: JSON.stringify({enabled})
    });
    const data = (await res.json()) as { enabled: boolean };
    return data.enabled;
  }
  return requireBackend().SetLaunchAtLogin(enabled);
}

export async function getRuntimeState(): Promise<RuntimeState> {
  if (isWebMode()) {
    const res = await remoteFetch('/api/runtime');
    return res.json();
  }
  return requireBackend().GetRuntimeState();
}

export async function getAnalyticsWeeklyStats(fromSec: number, toSec: number): Promise<AnalyticsWeeklyStats> {
  if (isWebMode()) {
    const res = await remoteFetch(`/api/analytics/weekly?fromSec=${fromSec}&toSec=${toSec}`);
    return res.json();
  }
  return requireBackend().GetAnalyticsWeeklyStats(fromSec, toSec);
}

export async function getAnalyticsSummary(fromSec: number, toSec: number): Promise<AnalyticsSummary> {
  if (isWebMode()) {
    const res = await remoteFetch(`/api/analytics/summary?fromSec=${fromSec}&toSec=${toSec}`);
    return res.json();
  }
  return requireBackend().GetAnalyticsSummary(fromSec, toSec);
}

export async function getAnalyticsTrendByDay(fromSec: number, toSec: number): Promise<AnalyticsTrend> {
  if (isWebMode()) {
    const res = await remoteFetch(`/api/analytics/trend?fromSec=${fromSec}&toSec=${toSec}`);
    return res.json();
  }
  return requireBackend().GetAnalyticsTrendByDay(fromSec, toSec);
}

export async function getAnalyticsBreakTypeDistribution(fromSec: number, toSec: number): Promise<AnalyticsBreakTypeDistribution> {
  if (isWebMode()) {
    const res = await remoteFetch(`/api/analytics/distribution?fromSec=${fromSec}&toSec=${toSec}`);
    return res.json();
  }
  return requireBackend().GetAnalyticsBreakTypeDistribution(fromSec, toSec);
}

export async function skipCurrentBreak(): Promise<RuntimeState> {
  if (isWebMode()) {
    const res = await remoteFetch('/api/skip-break', { method: 'POST' });
    return res.json();
  }
  return requireBackend().SkipCurrentBreak();
}

export async function postponeCurrentBreak(): Promise<RuntimeState> {
  if (isWebMode()) {
    const res = await remoteFetch('/api/skip-break', { method: 'POST' });
    return res.json();
  }
  return requireBackend().PostponeCurrentBreak();
}

export async function getNotificationCapability(): Promise<NotificationCapability> {
  if (isWebMode()) {
    const res = await remoteFetch('/api/notification/capability');
    return res.json();
  }
  return requireBackend().GetNotificationCapability();
}

export async function requestNotificationPermission(): Promise<NotificationCapability> {
  if (isWebMode()) {
    const res = await remoteFetch('/api/notification/request', { method: 'POST' });
    return res.json();
  }
  return requireBackend().RequestNotificationPermission();
}

export async function openNotificationSettings(): Promise<void> {
  if (isWebMode()) {
    await remoteFetch('/api/notification/open-settings', { method: 'POST' });
    return;
  }
  return requireBackend().OpenNotificationSettings();
}

export async function quitApp(): Promise<void> {
  if (isWebMode()) {
    await remoteFetch('/api/quit', { method: 'POST' });
    return;
  }
  const backend = requireBackend();
  if (!backend.Quit) {
    throw new Error('Pause backend bridge unavailable (window.go.app.App.Quit is missing).');
  }
  await backend.Quit();
}

export async function closeWindow(): Promise<void> {
  if (isWebMode()) {
    return;
  }
  const backend = requireBackend();
  if (!backend.CloseWindow) {
    throw new Error('Pause backend bridge unavailable (window.go.app.App.CloseWindow is missing).');
  }
  await backend.CloseWindow();
}

export async function forceBreak(): Promise<RuntimeState> {
  if (isWebMode()) {
    const res = await remoteFetch('/api/force-break', { method: 'POST' });
    return res.json();
  }
  return requireBackend().StartBreakNow?.() ?? requireBackend().GetRuntimeState();
}

export async function forceUnlock(): Promise<RuntimeState> {
  if (isWebMode()) {
    const res = await remoteFetch('/api/force-unlock', { method: 'POST' });
    return res.json();
  }
  return requireBackend().SkipCurrentBreak();
}

export async function takeScreenshot(): Promise<Blob> {
  const res = await remoteFetch('/screenshot');
  return res.blob();
}

export async function loginRemoteSession(token: string): Promise<void> {
  if (!isWebMode()) {
    return;
  }
  const value = String(token ?? '').trim();
  if (value === '') {
    throw new Error(ERR_REMOTE_AUTH_REQUIRED);
  }
  const res = await fetch('/api/auth/session', {
    method: 'POST',
    credentials: 'same-origin',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ token: value })
  });
  if (res.status === 401) {
    throw new Error(ERR_REMOTE_AUTH_INVALID);
  }
  if (!res.ok) {
    const body = await res.json().catch(() => ({ error: res.statusText }));
    throw new Error(body.error || `HTTP ${res.status}`);
  }
}

export async function logoutRemoteSession(): Promise<void> {
  if (!isWebMode()) {
    return;
  }
  await fetch('/api/auth/session', {
    method: 'DELETE',
    credentials: 'same-origin',
    headers: { 'Content-Type': 'application/json' }
  });
}

export function isRemoteWebMode(): boolean {
  return isWebMode();
}

export async function getRemoteAssetAccess(): Promise<RemoteAssetAccess> {
  return getRemoteRequestContext();
}

export async function getActivity(fromSec?: number, toSec?: number): Promise<ActivitySummary> {
  const params = new URLSearchParams();
  if (fromSec !== undefined) params.set('from', String(fromSec));
  if (toSec !== undefined) params.set('to', String(toSec));
  const qs = params.toString();
  const res = await remoteFetch(`/api/activity${qs ? '?' + qs : ''}`);
  return res.json();
}

export async function getAutoScreenshot(): Promise<boolean> {
  const res = await remoteFetch('/api/settings/auto-screenshot');
  const data = (await res.json()) as { enabled: boolean };
  return data.enabled;
}

export async function setAutoScreenshot(enabled: boolean): Promise<boolean> {
  const res = await remoteFetch('/api/settings/auto-screenshot', {
    method: 'POST',
    body: JSON.stringify({ enabled })
  });
  const data = (await res.json()) as { enabled: boolean };
  return data.enabled;
}

export function getShotUrl(name: string, access?: RemoteAssetAccess | null): string {
  const base = access?.baseUrl ?? '';
  const params = new URLSearchParams();
  if (access?.accessToken) {
    params.set('access_token', access.accessToken);
  }
  const qs = params.toString();
  return `${base}/shots/${encodeURIComponent(name)}${qs ? `?${qs}` : ''}`;
}

export async function getScreenshots(fromSec?: number, toSec?: number): Promise<ShotInfo[]> {
  const params = new URLSearchParams();
  if (fromSec !== undefined) params.set('from', String(fromSec));
  if (toSec !== undefined) params.set('to', String(toSec));
  const qs = params.toString();
  const res = await remoteFetch(`/api/screenshots${qs ? '?' + qs : ''}`);
  return res.json();
}

type RuntimeBrowserBridge = {
  BrowserOpenURL: (url: string) => void;
};

function requireBrowserBridge(): RuntimeBrowserBridge {
  const runtimeBridge = (window as unknown as { runtime?: RuntimeBrowserBridge }).runtime;
  if (!runtimeBridge?.BrowserOpenURL) {
    throw new Error('Pause runtime bridge unavailable (window.runtime.BrowserOpenURL is missing).');
  }
  return runtimeBridge;
}

function normalizeVersion(raw: string): number[] {
  const source = String(raw ?? '').trim().replace(/^v/i, '');
  if (source === '') {
    return [0, 0, 0];
  }
  const [core] = source.split('-', 1);
  const parts = core.split('.').map((part) => Number.parseInt(part, 10));
  return [parts[0] || 0, parts[1] || 0, parts[2] || 0];
}

function compareVersions(left: string, right: string): number {
  const a = normalizeVersion(left);
  const b = normalizeVersion(right);
  for (let index = 0; index < Math.max(a.length, b.length); index += 1) {
    const diff = (a[index] || 0) - (b[index] || 0);
    if (diff !== 0) {
      return diff;
    }
  }
  return 0;
}

function selectBestAsset(assets: UpdateAsset[] | undefined, os: string, arch: string) {
  const rows = Array.isArray(assets) ? assets.filter(Boolean) : [];
  const exact = rows.find((asset) => asset.os === os && asset.arch === arch && typeof asset.url === 'string');
  if (exact) {
    return exact;
  }
  const osOnly = rows.find((asset) => asset.os === os && typeof asset.url === 'string');
  if (osOnly) {
    return osOnly;
  }
  return rows.find((asset) => typeof asset.url === 'string') ?? null;
}

type UpdatesFeed = {
  release?: {
    version?: string;
    channel?: string;
    url?: string | null;
  };
  assets?: Array<{
    name?: string;
    path?: string;
    os?: string;
    arch?: string;
    kind?: string;
    sha256?: string;
    size?: number;
    url?: string | null;
  }>;
};

const APP_VERSION = import.meta.env.VITE_APP_VERSION || '0.0.0';
const UPDATES_URL = import.meta.env.VITE_UPDATES_URL || '';
const UPDATE_FETCH_TIMEOUT_MS = 10_000;

export const ERR_UPDATE_FEED_NOT_CONFIGURED = 'ERR_UPDATE_FEED_NOT_CONFIGURED';
export const ERR_UPDATE_FEED_TIMEOUT = 'ERR_UPDATE_FEED_TIMEOUT';
export const ERR_UPDATE_FEED_HTTP_PREFIX = 'ERR_UPDATE_FEED_HTTP_';
export const ERR_UPDATE_FETCH_FAILED = 'ERR_UPDATE_FETCH_FAILED';
export const ERR_UPDATE_DOWNLOAD_URL_MISSING = 'ERR_UPDATE_DOWNLOAD_URL_MISSING';

async function getPlatformInfo(): Promise<PlatformInfo> {
  if (isWebMode()) {
    return { os: 'web', arch: 'web' };
  }
  const backend = requireBackend();
  if (!backend.GetPlatformInfo) {
    return { os: 'unknown', arch: 'unknown' };
  }
  try {
    return await backend.GetPlatformInfo();
  } catch {
    return { os: 'unknown', arch: 'unknown' };
  }
}

export async function checkForUpdates(): Promise<UpdateCheckResult> {
  if (UPDATES_URL.trim() === '') {
    throw new Error(ERR_UPDATE_FEED_NOT_CONFIGURED);
  }

  const controller = new AbortController();
  const timeout = window.setTimeout(() => controller.abort(), UPDATE_FETCH_TIMEOUT_MS);
  let response: Response;
  try {
    response = await fetch(UPDATES_URL, {
      cache: 'no-store',
      signal: controller.signal
    });
  } catch (err) {
    if (err instanceof DOMException && err.name === 'AbortError') {
      throw new Error(ERR_UPDATE_FEED_TIMEOUT);
    }
    throw new Error(ERR_UPDATE_FETCH_FAILED);
  } finally {
    window.clearTimeout(timeout);
  }
  if (!response.ok) {
    throw new Error(`${ERR_UPDATE_FEED_HTTP_PREFIX}${response.status}`);
  }

  let payload: UpdatesFeed;
  try {
    payload = (await response.json()) as UpdatesFeed;
  } catch {
    throw new Error(ERR_UPDATE_FETCH_FAILED);
  }
  const latestVersion = String(payload.release?.version ?? '').trim();
  const { os, arch } = await getPlatformInfo();
  const selectedAsset = selectBestAsset(payload.assets as UpdateAsset[] | undefined, os, arch);

  return {
    currentVersion: APP_VERSION,
    latestVersion: latestVersion || null,
    channel: payload.release?.channel?.trim() || null,
    checkedAt: new Date().toISOString(),
    updateAvailable: latestVersion !== '' && compareVersions(latestVersion, APP_VERSION) > 0,
    releaseUrl: payload.release?.url?.trim() || selectedAsset?.url || null,
    selectedAsset
  };
}

export function openExternalURL(url: string): void {
  const target = String(url ?? '').trim();
  if (target === '') {
    throw new Error(ERR_UPDATE_DOWNLOAD_URL_MISSING);
  }
  if (isWebMode()) {
    window.open(target);
    return;
  }
  requireBrowserBridge().BrowserOpenURL(target);
}

type RuntimeBridge = {
  EventsOn: (eventName: string, callback: (payload: unknown) => void) => () => void;
};

function requireRuntimeBridge(): RuntimeBridge {
  const bridge = (window as unknown as { runtime?: RuntimeBridge }).runtime;
  if (!bridge?.EventsOn) {
    throw new Error('Pause runtime bridge unavailable (window.runtime.EventsOn is missing).');
  }
  return bridge;
}

export function onRuntimeTick(callback: (state: RuntimeState) => void): () => void {
  if (isWebMode()) {
    const interval = setInterval(async () => {
      try {
        const res = await remoteFetch('/api/runtime');
        const state = (await res.json()) as RuntimeState;
        callback(state);
      } catch { /* ignore polling errors */ }
    }, 1000);
    return () => clearInterval(interval);
  }
  const bridge = requireRuntimeBridge();
  return bridge.EventsOn('runtime:tick', (payload) => {
    const normalized = Array.isArray(payload) ? payload[0] : payload;
    callback(normalized as RuntimeState);
  });
}
