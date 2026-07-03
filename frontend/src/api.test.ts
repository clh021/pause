import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import type { RuntimeState, ReminderPatch, SettingsPatch, ReminderCreateInput } from './types';

vi.hoisted(() => {
  vi.stubEnv('VITE_UPDATES_URL', 'https://example.com/updates.json');
  vi.stubEnv('VITE_APP_VERSION', '1.0.0');
});

import * as api from './api';

function setWebMode(): void {
  vi.stubGlobal('window', { go: { app: { App: undefined } } });
}

function setNativeMode(): void {
  vi.stubGlobal('window', { go: { app: { App: {} } } });
}

function createMockResponse(data: unknown, ok = true, status = 200, statusText = 'OK') {
  return {
    ok,
    status,
    statusText,
    json: vi.fn().mockResolvedValue(data)
  } as unknown as Response;
}

const mockRuntimeState: RuntimeState = {
  currentSession: {
    status: 'break',
    reasons: [1],
    remainingSec: 30,
    canSkip: true,
    canPostpone: true
  },
  reminders: [],
  nextBreakReason: [],
  globalEnabled: true,
  timerMode: 'real_time',
  idleThresholdSec: 300,
  lastTickActive: true,
  showTrayCountdown: true,
  currentIdleSec: 0,
  overlaySkipAllowed: true,
  overlayNative: false,
  effectiveLanguage: 'en-US',
  effectiveTheme: 'light'
};

beforeEach(() => {
  vi.restoreAllMocks();
});

afterEach(() => {
  vi.unstubAllGlobals();
  vi.unstubAllEnvs();
  vi.useRealTimers();
});

// ---------------------------------------------------------------------------
// isWebMode (indirect — internal function, tested through exported API)
// ---------------------------------------------------------------------------
describe('isWebMode', () => {
  it('returns false when window.go.app.App exists — getSettings uses backend', async () => {
    setNativeMode();
    vi.spyOn(globalThis, 'fetch');

    await expect(api.getSettings()).rejects.toThrow();
    // fetch must NOT be called since isWebMode() is false
    expect(globalThis.fetch).not.toHaveBeenCalled();
  });

  it('returns true when window.go.app.App is undefined — getSettings uses fetch', async () => {
    setWebMode();
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(createMockResponse({}));

    await api.getSettings();
    expect(globalThis.fetch).toHaveBeenCalled();
  });
});

// ---------------------------------------------------------------------------
// webFetch (indirect — internal function)
// ---------------------------------------------------------------------------
describe('webFetch', () => {
  beforeEach(() => {
    setWebMode();
  });

  it('makes GET request to correct URL', async () => {
    const fetchSpy = vi.spyOn(globalThis, 'fetch').mockResolvedValue(createMockResponse({}));

    await api.getSettings();

    expect(fetchSpy).toHaveBeenCalledWith('/api/settings', expect.objectContaining({
      credentials: 'same-origin'
    }));
    expect((fetchSpy.mock.calls[0][1]?.headers as Headers).get('Content-Type')).toBe('application/json');
  });

  it('sets Content-Type header', async () => {
    const fetchSpy = vi.spyOn(globalThis, 'fetch').mockResolvedValue(createMockResponse({}));

    await api.getSettings();

    expect(fetchSpy.mock.calls[0][1]?.headers).toEqual(
      expect.any(Headers)
    );
    expect((fetchSpy.mock.calls[0][1]?.headers as Headers).get('Content-Type')).toBe('application/json');
  });

  it('returns parsed JSON on success', async () => {
    const settingsData = {
      enforcement: { overlaySkipAllowed: true },
      sound: { enabled: false },
      timer: { mode: 'real_time', idlePauseThresholdSec: 300 },
      ui: { showTrayCountdown: true, language: 'auto', theme: 'auto' }
    };
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(createMockResponse(settingsData));

    const result = await api.getSettings();

    expect(result).toEqual(settingsData);
  });

  it('throws on HTTP error', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      createMockResponse({ error: 'Server error' }, false, 500, 'Internal Server Error')
    );

    await expect(api.getSettings()).rejects.toThrow('Server error');
  });

  it('throws with status text on non-JSON error response', async () => {
    const badResponse = {
      ok: false,
      status: 502,
      statusText: 'Bad Gateway',
      json: vi.fn().mockRejectedValue(new Error('Not JSON'))
    } as unknown as Response;
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(badResponse);

    await expect(api.getSettings()).rejects.toThrow('Bad Gateway');
  });
});

// ---------------------------------------------------------------------------
// API functions in web mode
// ---------------------------------------------------------------------------
describe('API functions in web mode', () => {
  let fetchSpy: ReturnType<typeof vi.spyOn>;

  beforeEach(() => {
    setWebMode();
    fetchSpy = vi.spyOn(globalThis, 'fetch');
  });

  describe('getSettings', () => {
    it('calls GET /api/settings and returns parsed response', async () => {
      const data = {
        enforcement: { overlaySkipAllowed: true },
        sound: { enabled: false },
        timer: { mode: 'real_time', idlePauseThresholdSec: 300 },
        ui: { showTrayCountdown: true, language: 'auto', theme: 'auto' }
      };
      fetchSpy.mockResolvedValue(createMockResponse(data));

      const result = await api.getSettings();

      expect(fetchSpy).toHaveBeenCalledWith('/api/settings', expect.objectContaining({ credentials: 'same-origin' }));
      expect(result).toEqual(data);
    });
  });

  describe('updateSettings', () => {
    it('calls PATCH /api/settings/update with body and returns parsed response', async () => {
      const patch: SettingsPatch = { sound: { enabled: true } };
      const data = {
        enforcement: { overlaySkipAllowed: true },
        sound: { enabled: true },
        timer: { mode: 'real_time', idlePauseThresholdSec: 300 },
        ui: { showTrayCountdown: true, language: 'auto', theme: 'auto' }
      };
      fetchSpy.mockResolvedValue(createMockResponse(data));

      const result = await api.updateSettings(patch);

      expect(fetchSpy).toHaveBeenCalledWith(
        '/api/settings/update',
        expect.objectContaining({
          credentials: 'same-origin',
          method: 'PATCH',
          body: JSON.stringify(patch)
        })
      );
      expect(result).toEqual(data);
    });
  });

  describe('getReminders', () => {
    it('calls GET /api/reminders and returns parsed response', async () => {
      const data: import('./types').ReminderConfig[] = [
        { id: 1, name: 'Test', reminderType: 'rest', enabled: true, intervalSec: 1800, breakSec: 60 }
      ];
      fetchSpy.mockResolvedValue(createMockResponse(data));

      const result = await api.getReminders();

      expect(fetchSpy).toHaveBeenCalledWith('/api/reminders', expect.objectContaining({ credentials: 'same-origin' }));
      expect(result).toEqual(data);
    });
  });

  describe('createReminder', () => {
    it('calls POST /api/reminders/create with body and returns parsed response', async () => {
      const input: ReminderCreateInput = { name: 'New', intervalSec: 900, breakSec: 30 };
      const data: import('./types').ReminderConfig[] = [
        { id: 1, name: 'New', reminderType: 'rest', enabled: true, intervalSec: 900, breakSec: 30 }
      ];
      fetchSpy.mockResolvedValue(createMockResponse(data));

      const result = await api.createReminder(input);

      expect(fetchSpy).toHaveBeenCalledWith(
        '/api/reminders/create',
        expect.objectContaining({
          credentials: 'same-origin',
          method: 'POST',
          body: JSON.stringify(input)
        })
      );
      expect(result).toEqual(data);
    });
  });

  describe('updateReminder', () => {
    it('calls PUT /api/reminders/update/{id} with body and returns parsed response', async () => {
      const patch: ReminderPatch = { id: 5, name: 'Updated' };
      const data: import('./types').ReminderConfig[] = [
        { id: 5, name: 'Updated', reminderType: 'rest', enabled: true, intervalSec: 1800, breakSec: 60 }
      ];
      fetchSpy.mockResolvedValue(createMockResponse(data));

      const result = await api.updateReminder(patch);

      expect(fetchSpy).toHaveBeenCalledWith(
        '/api/reminders/update/5',
        expect.objectContaining({
          credentials: 'same-origin',
          method: 'PUT',
          body: JSON.stringify(patch)
        })
      );
      expect(result).toEqual(data);
    });
  });

  describe('deleteReminder', () => {
    it('calls DELETE /api/reminders/delete/{id} and returns parsed response', async () => {
      const data: import('./types').ReminderConfig[] = [
        { id: 3, name: 'Remaining', reminderType: 'notify', enabled: true, intervalSec: 3600, breakSec: 10 }
      ];
      fetchSpy.mockResolvedValue(createMockResponse(data));

      const result = await api.deleteReminder(3);

      expect(fetchSpy).toHaveBeenCalledWith(
        '/api/reminders/delete/3',
        expect.objectContaining({ credentials: 'same-origin', method: 'DELETE' })
      );
      expect(result).toEqual(data);
    });
  });

  describe('getLaunchAtLogin', () => {
    it('calls GET /api/settings/launch-at-login and returns boolean', async () => {
      fetchSpy.mockResolvedValue(createMockResponse({ enabled: true }));

      const result = await api.getLaunchAtLogin();

      expect(fetchSpy).toHaveBeenCalledWith(
        '/api/settings/launch-at-login',
        expect.objectContaining({ credentials: 'same-origin' })
      );
      expect(result).toBe(true);
    });
  });

  describe('setLaunchAtLogin', () => {
    it('calls POST /api/settings/launch-at-login/set with body and returns boolean', async () => {
      fetchSpy.mockResolvedValue(createMockResponse({ enabled: true }));

      const result = await api.setLaunchAtLogin(true);

      expect(fetchSpy).toHaveBeenCalledWith(
        '/api/settings/launch-at-login/set',
        expect.objectContaining({
          credentials: 'same-origin',
          method: 'POST',
          body: JSON.stringify({ enabled: true })
        })
      );
      expect(result).toBe(true);
    });
  });

  describe('getRuntimeState', () => {
    it('calls GET /api/runtime and returns parsed response', async () => {
      fetchSpy.mockResolvedValue(createMockResponse(mockRuntimeState));

      const result = await api.getRuntimeState();

      expect(fetchSpy).toHaveBeenCalledWith('/api/runtime', expect.objectContaining({ credentials: 'same-origin' }));
      expect(result).toEqual(mockRuntimeState);
    });
  });

  describe('skipCurrentBreak', () => {
    it('calls POST /api/skip-break and returns parsed response', async () => {
      fetchSpy.mockResolvedValue(createMockResponse(mockRuntimeState));

      const result = await api.skipCurrentBreak();

      expect(fetchSpy).toHaveBeenCalledWith(
        '/api/skip-break',
        expect.objectContaining({ credentials: 'same-origin', method: 'POST' })
      );
      expect(result).toEqual(mockRuntimeState);
    });
  });

  describe('postponeCurrentBreak', () => {
    it('calls POST /api/postpone-break and returns parsed response', async () => {
      fetchSpy.mockResolvedValue(createMockResponse(mockRuntimeState));

      const result = await api.postponeCurrentBreak();

      expect(fetchSpy).toHaveBeenCalledWith(
        '/api/postpone-break',
        expect.objectContaining({ credentials: 'same-origin', method: 'POST' })
      );
      expect(result).toEqual(mockRuntimeState);
    });
  });

  describe('getNotificationCapability', () => {
    it('calls GET /api/notification/capability and returns parsed response', async () => {
      const data = { permissionState: 'authorized', canRequest: false, canOpenSettings: true } as const;
      fetchSpy.mockResolvedValue(createMockResponse(data));

      const result = await api.getNotificationCapability();

      expect(fetchSpy).toHaveBeenCalledWith(
        '/api/notification/capability',
        expect.objectContaining({ credentials: 'same-origin' })
      );
      expect(result).toEqual(data);
    });
  });
});

// ---------------------------------------------------------------------------
// closeWindow, getPlatformInfo, openExternalURL
// ---------------------------------------------------------------------------
describe('closeWindow', () => {
  it('resolves immediately in web mode (no-op)', async () => {
    setWebMode();

    const result = await api.closeWindow();

    expect(result).toBeUndefined();
  });
});

describe('getPlatformInfo (indirect via checkForUpdates)', () => {
  it('returns {os:"web", arch:"web"} in web mode (verifies selected asset uses web platform)', async () => {
    vi.useFakeTimers();
    vi.stubGlobal('window', {
      go: { app: { App: undefined } },
        setTimeout: globalThis.setTimeout,
        clearTimeout: globalThis.clearTimeout
    });

    const updatePayload = {
      release: { version: '2.0.0', channel: 'stable' },
      assets: [
        { os: 'darwin', arch: 'amd64', url: 'https://example.com/mac.zip' },
        { os: 'web', arch: 'web', url: 'https://example.com/web.zip' },
        { os: 'linux', arch: 'amd64', url: 'https://example.com/linux.zip' }
      ]
    };

    vi.spyOn(globalThis, 'fetch').mockResolvedValue(createMockResponse(updatePayload));

    const result = await api.checkForUpdates();

    expect(result.latestVersion).toBe('2.0.0');
    expect(result.selectedAsset).toBeDefined();
    expect(result.selectedAsset!.os).toBe('web');
    expect(result.selectedAsset!.arch).toBe('web');
    expect(result.selectedAsset!.url).toBe('https://example.com/web.zip');
  });
});

describe('openExternalURL', () => {
  it('calls window.open in web mode', () => {
    const mockOpen = vi.fn();
    vi.stubGlobal('window', {
      go: { app: { App: undefined } },
      open: mockOpen
    });
    const targetUrl = 'https://example.com/download';

    api.openExternalURL(targetUrl);

    expect(mockOpen).toHaveBeenCalledWith(targetUrl);
  });

  it('throws on empty URL even in web mode', () => {
    setWebMode();

    expect(() => api.openExternalURL('')).toThrow();
    expect(() => api.openExternalURL('  ')).toThrow();
  });
});

// ---------------------------------------------------------------------------
// onRuntimeTick
// ---------------------------------------------------------------------------
describe('onRuntimeTick in web mode', () => {
  beforeEach(() => {
    vi.useFakeTimers();
    setWebMode();
  });

  it('starts an interval that calls /api/runtime and passes state to callback', async () => {
    const fetchSpy = vi.spyOn(globalThis, 'fetch').mockResolvedValue(createMockResponse(mockRuntimeState));
    const callback = vi.fn();

    const cleanup = api.onRuntimeTick(callback);

    // Not called immediately
    expect(callback).not.toHaveBeenCalled();

    await vi.advanceTimersByTimeAsync(1000);

    expect(fetchSpy).toHaveBeenCalledWith('/api/runtime', expect.objectContaining({ credentials: 'same-origin' }));
    expect(callback).toHaveBeenCalledTimes(1);
    expect(callback).toHaveBeenCalledWith(mockRuntimeState);

    cleanup();
  });

  it('cleanup function clears the interval', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(createMockResponse(mockRuntimeState));
    const callback = vi.fn();

    const cleanup = api.onRuntimeTick(callback);

    await vi.advanceTimersByTimeAsync(1000);
    expect(callback).toHaveBeenCalledTimes(1);

    cleanup();

    await vi.advanceTimersByTimeAsync(3000);
    // Should still be 1 because interval was cleared
    expect(callback).toHaveBeenCalledTimes(1);
  });
});
