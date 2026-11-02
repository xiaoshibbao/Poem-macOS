package launcher

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"poem/internal/wine"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const luaMaxBytes = 4 * 1024 * 1024

// Missing disabled-script settings preserve the previous all-enabled behavior.
func enabledLuaScripts(cfg Settings) []string {
	result := []string{}
	for _, path := range cfg.LuaScripts {
		disabled := false
		for _, off := range cfg.LuaDisabledScripts {
			if strings.EqualFold(filepath.Clean(path), filepath.Clean(off)) {
				disabled = true
				break
			}
		}
		if !disabled {
			result = append(result, path)
		}
	}
	return result
}

type LuaSessionStatus struct {
	PID     uint32 `json:"pid"`
	ID      string `json:"id"`
	State   string `json:"state"`
	Message string `json:"message"`
}

func readLuaStatus(pid uint32, dir string) LuaSessionStatus {
	result := LuaSessionStatus{PID: pid, State: "idle", Message: "尚未加载；进入游戏后点击执行"}
	data, err := os.ReadFile(filepath.Join(dir, "lua-status.txt"))
	if err != nil {
		return result
	}
	fields := strings.SplitN(string(data), "\n", 3)
	if len(fields) != 3 {
		return result
	}
	result.ID, result.State, result.Message = fields[0], fields[1], fields[2]
	return result
}

func (s *Service) GetLuaSessions() []LuaSessionStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := []LuaSessionStatus{}
	for pid, session := range s.sessions {
		result = append(result, readLuaStatus(pid, session.moduleDir))
	}
	sort.Slice(result, func(i, j int) bool { return result[i].PID < result[j].PID })
	return result
}

// Snapshot file contents at click time. Never execute arbitrary Lua in the
// launcher, and never send scripts or credentials to any server.
func buildLuaBundle(paths []string) ([]byte, error) {
	if len(paths) == 0 || len(paths) > 16 {
		return nil, fmt.Errorf("请选择 1–16 个 Lua 脚本")
	}
	var bundle strings.Builder
	bundle.WriteString("assert(CS and CS.RPG and CS.UnityEngine, 'Game xLua environment is not ready; enter the game and retry')\nlocal __poem_errors = {}\n")
	total := 0
	for i, path := range paths {
		if !strings.EqualFold(filepath.Ext(path), ".lua") {
			return nil, fmt.Errorf("仅支持 .lua 脚本：%s", filepath.Base(path))
		}
		f, err := os.Open(path)
		if err != nil {
			return nil, fmt.Errorf("读取脚本 %s：%w", filepath.Base(path), err)
		}
		b, err := io.ReadAll(io.LimitReader(f, luaMaxBytes+1))
		f.Close()
		if err != nil {
			return nil, err
		}
		total += len(b)
		if total > luaMaxBytes {
			return nil, fmt.Errorf("脚本总大小不能超过 4 MiB")
		}
		b = bytes.TrimPrefix(b, []byte{0xef, 0xbb, 0xbf})
		if !utf8.Valid(b) || bytes.IndexByte(b, 0) >= 0 {
			return nil, fmt.Errorf("脚本必须是 UTF-8 文本：%s", filepath.Base(path))
		}
		// Each script gets its normal chunk-local scope, while _G remains shared.
		fmt.Fprintf(&bundle, "do\nlocal function __poem_script_%d()\n", i)
		bundle.Write(b)
		fmt.Fprintf(&bundle, "\nend\nlocal ok, failure = pcall(__poem_script_%d)\nif not ok then table.insert(__poem_errors, %s .. ': ' .. tostring(failure)) end\nend\n", i, luaString(filepath.Base(path)))
	}
	bundle.WriteString("if #__poem_errors > 0 then error(table.concat(__poem_errors, '\\n')) end\n")
	if bundle.Len() > luaMaxBytes {
		return nil, fmt.Errorf("打包后的脚本超过 4 MiB")
	}
	return []byte(bundle.String()), nil
}

func luaString(value string) string {
	var out strings.Builder
	out.WriteByte('"')
	for _, b := range []byte(value) {
		fmt.Fprintf(&out, "\\%03d", b)
	}
	out.WriteByte('"')
	return out.String()
}

func (s *Service) RunLua(pid uint32, paths []string) (LuaSessionStatus, error) {
	return s.queueLua(pid, paths, false)
}

func (s *Service) queueLua(pid uint32, paths []string, automatic bool) (LuaSessionStatus, error) {
	bundle, err := buildLuaBundle(paths)
	if err != nil {
		return LuaSessionStatus{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.sessions[pid]
	if !ok {
		return LuaSessionStatus{}, fmt.Errorf("请选择当前由 Poem 启动的游戏窗口")
	}
	dir := session.moduleDir
	current := readLuaStatus(pid, dir)
	if current.State == "queued" || current.State == "running" || current.State == "scheduled" {
		return current, fmt.Errorf("上一批脚本尚未执行完毕")
	}
	request := filepath.Join(dir, "lua-request.bin")
	if _, err := os.Stat(request); err == nil {
		return current, fmt.Errorf("脚本已提交，正在等待游戏 Lua 环境")
	}
	if !session.luaLoaded {
		path := filepath.Join(dir, "Poem.Lua.dll")
		if err := writeModule(path, s.lua); err != nil {
			return current, err
		}
		helper, err := s.helperPath()
		if err != nil {
			return current, err
		}
		data, _ := dataDir()
		logf := func(format string, args ...any) { writeLaunchLog(data, format, args...) }
		if err := attachModule(session.wine, helper, pid, path, logf); err != nil {
			return current, fmt.Errorf("加载 Lua 模块失败：%w", err)
		}
		session.luaLoaded = true
	}
	id := uint64(time.Now().UnixNano())
	if automatic {
		// The game writes this file itself, so it needs the path as Wine sees it.
		bundle = deferredLuaBundle(bundle, wine.WindowsPath(session.wine.Prefix, filepath.Join(dir, "lua-status.txt")), id)
	}
	if len(bundle) > luaMaxBytes {
		return current, fmt.Errorf("打包后的脚本超过 4 MiB")
	}
	data := binary.LittleEndian.AppendUint64(nil, id)
	data = binary.LittleEndian.AppendUint32(data, uint32(len(bundle)))
	data = append(data, bundle...)
	tmp := filepath.Join(dir, "lua-request.tmp")
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return current, err
	}
	if err := os.Rename(tmp, request); err != nil {
		_ = os.Remove(tmp)
		return current, err
	}
	return LuaSessionStatus{PID: pid, ID: strconv.FormatUint(id, 10), State: "queued", Message: "已提交，等待游戏主线程执行"}, nil
}

// StartCoroutine schedules the wait and script execution on Unity's own
// coroutine loop. No sleep or game API call occurs on a remote worker thread.
func deferredLuaBundle(bundle []byte, statusPath string, id uint64) []byte {
	prefix := fmt.Sprintf(`-- POEM_DEFERRED
local __poem_status_path = %s
local function __poem_report(state, message)
 local text = %s .. '\n' .. state .. '\n' .. tostring(message):gsub('[\r\n]', ' ')
 CS.System.IO.File.WriteAllText(__poem_status_path, text)
end
local function __poem_ready()
 local sys = CS.System
 local flags = sys.Reflection.BindingFlags.__CastFrom(56)
 local fields = typeof(CS.RPG.Client.GlobalVars):GetFields(flags)
 for i = 0, fields.Length - 1 do
  local field = fields:GetValue(i)
  xlua.cast(field, typeof(sys.Reflection.FieldInfo))
  if field.FieldType == typeof(CS.RPG.Client.GamePhaseManager) then
   local manager = field:GetValue(nil)
   if manager == nil then return false, 'Waiting for GamePhaseManager' end
   xlua.cast(manager, typeof(CS.RPG.Client.GamePhaseManager))
   local phase = manager:GetCurrentPhase()
   if phase == nil then return false, 'Waiting for the current game phase' end
   local name = tostring(phase:GetType().Name)
   local ready = name == 'EnterGamePhase' or name == 'AdventurePhase' or name == 'AdventureGamePhase' or name == 'FieldGamePhase' or name == 'BattlePhase' or name == 'BattleGamePhase'
   return ready, 'Current game phase: ' .. name
  end
 end
 return false, 'GamePhaseManager field not found'
end
local function __poem_run()
`, luaString(statusPath), luaString(strconv.FormatUint(id, 10)))
	suffix := `
end
local util = require('xlua.util')
CS.RPG.Client.CoroutineUtils.StartCoroutine(util.cs_generator(function()
 local deadline = CS.UnityEngine.Time.realtimeSinceStartup + 300
 local previousReason
 while true do
  local ok, ready, reason = pcall(__poem_ready)
  if ok and ready then break end
  local message = ok and (reason or 'Waiting for game data') or tostring(ready)
  if message ~= previousReason then __poem_report('scheduled', message); previousReason = message end
  if CS.UnityEngine.Time.realtimeSinceStartup > deadline then
   __poem_report('error', 'Timed out waiting for game data: ' .. tostring(previousReason))
   return
  end
  for _ = 1, 30 do coroutine.yield(nil) end
 end
 __poem_report('running', 'Executing automatic Lua scripts')
 local ok, failure = pcall(__poem_run)
 __poem_report(ok and 'success' or 'error', ok and 'Automatic Lua scripts executed' or failure)
end))
`
	return []byte(prefix + string(bundle) + suffix)
}
