// Static data shared by the UI: version, labels, presets.
export const VERSION = '2.0.0-macos'
export const languageNames: Record<string,string> = {cn:'中文',en:'English',jp:'日本語',kr:'한국어'}
export const keyNames: Record<number, string> = { 32: 'Space', 9: 'Tab', 16: 'Shift', 17: 'Ctrl', 18: 'Alt', 20: 'Caps Lock', 13: 'Enter', 27: 'Esc', 192: '`', 1: '鼠标左键', 2: '鼠标右键', 4: '鼠标中键', 5: '鼠标侧键 1', 6: '鼠标侧键 2' }
export function keyName(key: number) { return keyNames[key] || (key >= 112 && key <= 123 ? `F${key - 111}` : key >= 48 && key <= 90 ? String.fromCharCode(key) : `VK 0x${key.toString(16).toUpperCase()}`) }
export const servers = { local: 'http://127.0.0.1:21000', poem: 'http://43.142.131.90:21000/' }
export const serverLabels = { local: '本地服务器', poem: 'Poem', custom: '自定义' }
export type ServerMode = keyof typeof serverLabels
export function serverMode(url: string): ServerMode { return url === servers.local ? 'local' : url.replace(/\/$/, '') === servers.poem.replace(/\/$/, '') ? 'poem' : 'custom' }
export const fpsPresets = [60, 120, 144, 240]
export type Tab = 'home' | 'env' | 'game' | 'proxy' | 'language' | 'lua'
