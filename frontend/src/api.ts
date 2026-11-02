import { Call, Events } from '@wailsio/runtime'

export interface Settings {
 loginChannel: 'private' | 'official' | 'bilibili';
 luaEnabled: boolean; luaScripts: string[]; luaDisabledScripts: string[];
 gamePath: string; proxyEnabled: boolean; serverURL: string; proxyPort: number;
 rsaPatch: boolean; rsaKey: string; webRedirect: boolean; webHosts: string;
 timeStop: boolean; timeStopKey: number; unlockFPS: boolean; targetFPS: number;
 multiInstance: boolean; touchUI: boolean; instances: number; removeBlur: boolean; textLanguage: string; voiceLanguage: string;
 winePath: string; winePrefix: string; wineEnv: string;
}
export interface Status { running: number; ports: number[]; message: string }
export interface LuaStatus { pid: number; id: string; state: string; message: string }
export interface Environment {
 runtimes: { name: string; path: string }[]; prefixes: { name: string; path: string }[];
 modules: { name: string; feature: string; present: boolean }[]; modulesDir: string; dataDir: string;
}
export const defaults: Settings = {
 loginChannel: 'private',
 luaEnabled: false, luaScripts: [], luaDisabledScripts: [],
 gamePath: '', proxyEnabled: true, serverURL: 'http://127.0.0.1:21000', proxyPort: 8080,
 rsaPatch: true, rsaKey: '', webRedirect: true, webHosts: '', timeStop: true,
 timeStopKey: 32, unlockFPS: true, targetFPS: 120, multiInstance: true, touchUI: false, instances: 1, removeBlur: false, textLanguage: '', voiceLanguage: '',
 winePath: '', winePrefix: '', wineEnv: '',
}
export const preview = new URLSearchParams(location.search).has('preview')
const call = <T,>(method: string, ...args: unknown[]): Promise<T> => Call.ByName(`poem/internal/launcher.Service.${method}`, ...args)
export const api = {
 prepareBilibili: async (): Promise<string> => { if (preview) throw new Error('请在 Poem 中下载渠道组件'); return call('PrepareBilibiliSDK') },
 luaSessions: async (): Promise<LuaStatus[]> => preview ? [] : call('GetLuaSessions'),
 runLua: async (pid: number, paths: string[]): Promise<LuaStatus> => { if (preview) throw new Error('请在 Poem 中执行脚本'); return call('RunLua', pid, paths) },
 pickLua: async (): Promise<string> => preview ? '' : Call.ByName('poem/internal/dialog.Service.PickFile', 'lua', ''),
 load: async (): Promise<Settings> => preview ? { ...defaults, ...JSON.parse(localStorage.getItem('poem-preview') || '{}') } : call('LoadSettings'),
 save: async (settings: Settings): Promise<void> => { if (preview) { localStorage.setItem('poem-preview', JSON.stringify(settings)); return }; await call('SaveSettings', settings) },
 start: async (settings: Settings): Promise<Status> => { if (preview) throw new Error('浏览器预览无法启动游戏，请使用 Poem.app'); return call('Start', settings) },
 languageInfo: async (path: string): Promise<{text: string; voice: string; version: string}> => preview ? {text:'cn',voice:'cn',version:'界面预览'} : call('GetLanguageInfo', path),
 restoreLanguage: async (path: string): Promise<void> => { if (preview) return; await call('RestoreLanguage', path) },
 status: async (): Promise<Status> => preview ? { running: 0, ports: [], message: '' } : call('GetStatus'),
 pick: async (start: string): Promise<string> => { if (preview) throw new Error('请在 Poem.app 中选择游戏文件'); return Call.ByName('poem/internal/dialog.Service.PickFile', 'exe', start) },
 pickWine: async (): Promise<string> => { if (preview) throw new Error('请在 Poem.app 中选择 Wine'); return Call.ByName('poem/internal/dialog.Service.PickFile', 'wine', '/Applications') },
 pickPrefix: async (start: string): Promise<string> => { if (preview) throw new Error('请在 Poem.app 中选择 Wine 容器'); return Call.ByName('poem/internal/dialog.Service.PickFolder', start) },
 environment: async (): Promise<Environment> => preview ? { runtimes: [{ name: 'CrossOver', path: '/Applications/CrossOver.app/Contents/SharedSupport/CrossOver/bin/wine' }], prefixes: [{ name: 'CrossOver · StarRail', path: '~/Library/Application Support/CrossOver/Bottles/StarRail' }], modules: [{ name: 'ProxyHook.dll', feature: '私服网络代理', present: true }, { name: 'GameFeatures.dll', feature: '时停 / 帧率 / 多开 / 移动端 UI', present: false }, { name: 'VisualPatch.dll', feature: '角色去虚化', present: false }], modulesDir: '~/Library/Application Support/Poem/modules', dataDir: '~/Library/Application Support/Poem' } : call('GetEnvironment'),
 openFolder: async (path: string) => { if (!preview) await Call.ByName('poem/internal/dialog.Service.OpenFolder', path) },
 minimize: () => preview ? Promise.resolve() : Call.ByName('poem/internal/window.Service.Minimize'),
 close: () => preview ? Promise.resolve() : Call.ByName('poem/internal/window.Service.Close'),
 subscribe: (fn: (s: Status) => void) => preview ? () => {} : Events.On('poem:status', event => fn(event.data as Status)),
}

// Wails errors arrive as JSON strings; surface just their message.
export function errorMessage(error: unknown): string {
 const message = error instanceof Error ? error.message : typeof error === 'object' && error !== null && 'message' in error ? String(error.message) : String(error)
 try { const parsed = JSON.parse(message); return parsed.message || message } catch { return message }
}
