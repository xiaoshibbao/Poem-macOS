package launcher

import (
	"encoding/binary"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnabledLuaScriptsPreservesOrderAndOldSettings(t *testing.T) {
	cfg := Settings{LuaScripts: []string{`C:\lua\a.lua`, `C:\lua\b.lua`, `C:\lua\c.lua`}}
	if got := enabledLuaScripts(cfg); len(got) != 3 {
		t.Fatal("old settings must enable all scripts")
	}
	cfg.LuaDisabledScripts = []string{`c:\LUA\b.lua`}
	got := enabledLuaScripts(cfg)
	if len(got) != 2 || got[0] != cfg.LuaScripts[0] || got[1] != cfg.LuaScripts[2] {
		t.Fatalf("wrong selection/order: %v", got)
	}
	cfg.LuaDisabledScripts = cfg.LuaScripts
	if len(enabledLuaScripts(cfg)) != 0 {
		t.Fatal("disabled scripts still selected")
	}
}

func TestLuaBundleValidation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "脚本.lua")
	if err := os.WriteFile(path, []byte("\xef\xbb\xbflocal x = 1\nreturn x"), 0600); err != nil {
		t.Fatal(err)
	}
	bundle, err := buildLuaBundle([]string{path})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(bundle), "local x = 1") || strings.Contains(string(bundle), "\ufeff") {
		t.Fatal("invalid bundle")
	}
	if _, err = buildLuaBundle(nil); err == nil {
		t.Fatal("empty scripts accepted")
	}
	if _, err = buildLuaBundle([]string{filepath.Join(dir, "bad.exe")}); err == nil {
		t.Fatal("non-Lua accepted")
	}
	if err = os.WriteFile(path, []byte{0xff, 0x00}, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = buildLuaBundle([]string{path}); err == nil {
		t.Fatal("invalid text accepted")
	}
	if _, err = New(Modules{}).RunLua(123, []string{path}); err == nil {
		t.Fatal("unknown session accepted")
	}
}

func TestNativeLuaExecutesOnGameThreadAndRestoresStack(t *testing.T) {
	helper := os.Getenv("POEM_LUA_FIXTURE")
	if helper == "" {
		t.Skip("native Lua fixture not configured")
	}
	for _, failure := range []bool{false, true} {
		dir := t.TempDir()
		bridge := filepath.Join(dir, "Poem.Lua.dll")
		b, err := os.ReadFile("../../assets/LuaEntry.dll")
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(bridge, b, 0600); err != nil {
			t.Fatal(err)
		}
		script := "print('fixture')"
		want := "success"
		if failure {
			script = "FAIL_LUA"
			want = "error"
		}
		request := binary.LittleEndian.AppendUint64(nil, 123)
		request = binary.LittleEndian.AppendUint32(request, uint32(len(script)))
		request = append(request, script...)
		if err = os.WriteFile(filepath.Join(dir, "lua-request.bin"), request, 0600); err != nil {
			t.Fatal(err)
		}
		report := filepath.Join(dir, "report.txt")
		out, err := exec.Command(helper, filepath.Join(filepath.Dir(helper), "lua-fixture", "xluau.dll"), bridge, report).CombinedOutput()
		if err != nil {
			t.Fatalf("fixture failed: %v %s", err, out)
		}
		b, err = os.ReadFile(report)
		if err != nil {
			t.Fatal(err)
		}
		if string(b) != "executions=1 wrongThread=0 top=7" {
			t.Fatalf("Lua thread/stack mismatch: %s", b)
		}
		state := readLuaStatus(0, dir)
		if state.State != want || state.ID != "123" {
			t.Fatalf("unexpected status: %+v", state)
		}
		t.Logf("%s: %s", want, b)
	}
}

func TestRealXLuaDeferredExecution(t *testing.T) {
	helper, dll := os.Getenv("POEM_LUA_COMPILER"), os.Getenv("POEM_XLUA_DLL")
	if helper == "" || dll == "" {
		t.Skip("isolated game xLua compiler not configured")
	}
	mock := `
local phaseName = 'LoginGamePhase'
local report, task
CS = { UnityEngine = { Camera = { main = nil }, Time = { realtimeSinceStartup = 0 } }, RPG = { Client = {} }, System = {} }
typeof = function(value) return value end
xlua = { cast = function() end }
CS.System.Reflection = { BindingFlags = { __CastFrom = function(value) assert(value == 56); return value end }, FieldInfo = {} }
CS.System.Enum = { Parse = function() return 1 end }
CS.System.IO = { File = { WriteAllText = function(path, text) report = text end } }
CS.RPG.GameCore = { AvatarSkillExcelTable = { GetData = function() error('Readiness must never trigger table loading') end } }
CS.RPG.Client.GamePhaseManager = {}
local manager = { GetCurrentPhase = function() return { GetType = function() return { Name = phaseName } end } end }
local field = { FieldType = CS.RPG.Client.GamePhaseManager, GetValue = function() return manager end }
CS.RPG.Client.GlobalVars = { GetFields = function() return { Length = 1, GetValue = function() return field end } end }
CS.RPG.Client.CoroutineUtils = { StartCoroutine = function(thread) task = thread end }
require = function(name) assert(name == 'xlua.util'); return { cs_generator = function(fn) return coroutine.create(fn) end } end
`
	for _, mode := range []string{"success", "error", "timeout"} {
		body := []byte("__poem_counter = (__poem_counter or 0) + 1")
		if mode == "error" {
			body = []byte("error('expected script error')")
		}
		code := mock + string(deferredLuaBundle(body, "ignored-by-mock", 123)) + `
assert(task, 'automatic task not scheduled')
for i=1,31 do local ok,err=coroutine.resume(task); assert(ok,err) end
assert(__poem_counter == nil and report:find('scheduled', 1, true), 'script ran before game data was ready')
`
		if mode == "timeout" {
			code += "CS.UnityEngine.Time.realtimeSinceStartup = 301\n"
		} else {
			code += "phaseName = 'EnterGamePhase'\n"
		}
		code += `for i=1,31 do if coroutine.status(task) ~= 'dead' then local ok,err=coroutine.resume(task); assert(ok,err) end end
assert(coroutine.status(task) == 'dead', 'automatic task did not finish')
`
		switch mode {
		case "success":
			code += "assert(__poem_counter == 1 and report:find('123\\nsuccess\\n', 1, true), 'success not reported')\n"
		case "error":
			code += "assert(report:find('123\\nerror\\n', 1, true) and report:find('expected script error', 1, true), 'error not reported')\n"
		case "timeout":
			code += "assert(__poem_counter == nil and report:find('Timed out', 1, true), 'timeout not reported')\n"
		}
		path := filepath.Join(t.TempDir(), "auto.lua")
		if err := os.WriteFile(path, []byte(code), 0600); err != nil {
			t.Fatal(err)
		}
		out, err := exec.Command(helper, dll, path, "execute-mock").CombinedOutput()
		if err != nil {
			t.Fatalf("%s: %v %s", mode, err, out)
		}
		t.Logf("%s: %s", mode, out)
	}
}

func TestUserLuaSyntax(t *testing.T) {
	helper, dll, samples := os.Getenv("POEM_LUA_COMPILER"), os.Getenv("POEM_XLUA_DLL"), os.Getenv("POEM_LUA_SAMPLES")
	if helper == "" || dll == "" || samples == "" {
		t.Skip("isolated game xLua compiler or sample scripts (POEM_LUA_SAMPLES, ';'-separated) not configured")
	}
	paths := strings.Split(samples, ";")
	for _, path := range paths {
		if _, err := os.Stat(path); err != nil {
			t.Skip("user example files unavailable")
		}
		out, err := exec.Command(helper, dll, path).CombinedOutput()
		if err != nil {
			t.Fatalf("%s: %v %s", path, err, out)
		}
	}
	bundle, err := buildLuaBundle(paths)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "bundled.lua")
	if err = os.WriteFile(path, deferredLuaBundle(bundle, "isolated-status", 123), 0600); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(helper, dll, path).CombinedOutput()
	if err != nil {
		t.Fatalf("combined automatic scripts: %v %s", err, out)
	}
	t.Logf("all three scripts and automatic bundle: %s", out)
}
