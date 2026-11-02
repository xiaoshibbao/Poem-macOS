# Local xLua bridge

The launcher loads this bridge into one of its own live game sessions after
game launch, then submits a bounded UTF-8 source snapshot via that session's
private runtime directory. It never executes user Lua in the launcher or sends
it to a server. No game files are changed.

The bridge finds the loaded xluau.dll with the required exported APIs and uses
MinHook v1.3.4 to hook lua_pcall. After an outermost successful call on the
UnityWndClass window thread, it executes at most one pending source chunk.
TLS prevents reentry. The existing Lua stack, including the original call's
results, is restored after both success and Lua errors. File polling runs on a
worker; Lua API calls never run there. Missing exports/hook failures are
reported rather than patched via guessed offsets. The bridge remains loaded
until game exit; disabling automatic execution does not undo existing scripts.

Automatic mode schedules a coroutine using the game's xlua.util and
RPG.Client.CoroutineUtils. It waits for a camera and the avatar skill-table row
used by the supplied Feixiao example (122004, level 1), with a five-minute
timeout, then executes the configured scripts in order once. It does not use
phase-name reflection. Waiting reasons and lookup errors are exposed in status.
Individual scripts run under pcall; errors are aggregated and reported. Native
faults, infinite loops and bugs inside arbitrary scripts cannot be caught by
Lua pcall. No forced thread cancellation or module unload is attempted.

Build with scripts/build-lua.ps1. MinHook upstream:
https://github.com/TsudaKageyu/minhook/tree/v1.3.4
Its complete license is retained in minhook/LICENSE.txt and distributed under
dist/third-party/MinHook-LICENSE.txt. No third-party source was copied; xLua
API declarations were consulted for the exported function signatures.

Tests use a disposable hidden-window process and fake xLua exports to verify
main-thread execution, error reporting and exact stack restoration. A second
isolated process uses the actual xLua DLL only to compile user examples and
execute a mocked scene lifecycle. The agent has not launched the real game.

Environment variables for Go tests:
POEM_LUA_FIXTURE=.../qa/Poem-lua-fixture.exe
POEM_LUA_COMPILER=.../qa/Poem-lua-compiler.exe
POEM_XLUA_DLL=.../StarRail_Data/Plugins/x86_64/xluau.dll
