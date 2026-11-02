#define WIN32_LEAN_AND_MEAN
#include <windows.h>
#include <tlhelp32.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include "minhook/include/MinHook.h"

// All Lua API calls happen inside the game's outermost successful lua_pcall,
// on its Unity window thread. The worker only handles files and hook setup.
typedef int (*PCall)(void *,int,int,int);
typedef int (*LoadBuffer)(void *,const char *,size_t,const char *);
typedef int (*GetTop)(void *);
typedef void (*SetTop)(void *,int);
typedef const char *(*ToString)(void *,int,size_t *);
static PCall original_pcall;
static LoadBuffer loadbuffer;
static GetTop gettop;
static SetTop settop;
static ToString tostring;
static DWORD depth_slot=TLS_OUT_OF_INDEXES;
static volatile LONG main_thread;
static SRWLOCK queue_lock=SRWLOCK_INIT, status_lock=SRWLOCK_INIT;
static char *pending;
static DWORD pending_size;
static uint64_t pending_id;
static WCHAR root[32768];
static HINSTANCE self_module;
#define MAX_SCRIPT (4*1024*1024)

static void path_for(WCHAR *out,const WCHAR *name) {
 _snwprintf(out,32768,L"%ls\\%ls",root,name);out[32767]=0;
}
static void status(uint64_t id,const char *state,const char *message) {
 WCHAR path[32768],tmp[32768];
 char text[2304],clean[2048];size_t i;
 for(i=0;message && message[i] && i<sizeof(clean)-1;i++)clean[i]=(message[i]=='\r'||message[i]=='\n')?' ':message[i];
 clean[i]=0;
 int length=snprintf(text,sizeof(text),"%llu\n%s\n%s",(unsigned long long)id,state,clean);
 path_for(path,L"lua-status.txt");path_for(tmp,L"lua-status.tmp");
 AcquireSRWLockExclusive(&status_lock);
 HANDLE f=CreateFileW(tmp,GENERIC_WRITE,FILE_SHARE_READ,NULL,CREATE_ALWAYS,FILE_ATTRIBUTE_NORMAL,NULL);
 if(f!=INVALID_HANDLE_VALUE){DWORD written;WriteFile(f,text,(DWORD)length,&written,NULL);CloseHandle(f);MoveFileExW(tmp,path,MOVEFILE_REPLACE_EXISTING);}
 ReleaseSRWLockExclusive(&status_lock);
}
static int hook_pcall(void *L,int args,int results,int error_function) {
 uintptr_t depth=(uintptr_t)TlsGetValue(depth_slot);
 TlsSetValue(depth_slot,(void *)(depth+1));
 int result=original_pcall(L,args,results,error_function);
 if(!depth && !result && GetCurrentThreadId()==(DWORD)InterlockedCompareExchange(&main_thread,0,0)) {
  char *script=NULL;DWORD size=0;uint64_t id=0;
  if(TryAcquireSRWLockExclusive(&queue_lock)) {
   script=pending;size=pending_size;id=pending_id;pending=NULL;
   ReleaseSRWLockExclusive(&queue_lock);
  }
  if(script) {
   int deferred=size>=16 && !memcmp(script,"-- POEM_DEFERRED",16);
   status(id,deferred?"scheduled":"running",deferred?"Waiting for a playable scene":"Executing on the game main thread");
   int top=gettop(L);
   int error=loadbuffer(L,script,size,"@Poem/local-scripts");
   if(!error)error=original_pcall(L,0,0,0);
   if(error){size_t length=0;const char *message=tostring(L,-1,&length);status(id,"error",message?message:"Lua execution failed");}
   else if(!deferred)status(id,"success","Lua scripts executed");
   settop(L,top);
   HeapFree(GetProcessHeap(),0,script);
  }
 }
 TlsSetValue(depth_slot,(void *)depth);
 return result;
}
static BOOL CALLBACK find_window(HWND hwnd,LPARAM unused) {
 DWORD pid;DWORD thread=GetWindowThreadProcessId(hwnd,&pid);WCHAR klass[128];
 if(pid==GetCurrentProcessId() && GetClassNameW(hwnd,klass,128) && !wcscmp(klass,L"UnityWndClass")) {
  InterlockedExchange(&main_thread,(LONG)thread);return FALSE;
 }
 return TRUE;
}
static HMODULE find_xlua(void) {
 HANDLE snapshot=CreateToolhelp32Snapshot(TH32CS_SNAPMODULE,GetCurrentProcessId());
 if(snapshot==INVALID_HANDLE_VALUE)return NULL;
 MODULEENTRY32W entry;ZeroMemory(&entry,sizeof(entry));entry.dwSize=sizeof(entry);HMODULE result=NULL;
 if(Module32FirstW(snapshot,&entry))do {
  if(!_wcsicmp(entry.szModule,L"xluau.dll") && GetProcAddress(entry.hModule,"xluaL_loadbuffer") && GetProcAddress(entry.hModule,"lua_pcall")) {result=entry.hModule;break;}
 }while(Module32NextW(snapshot,&entry));
 CloseHandle(snapshot);return result;
}
static void read_request(void) {
 WCHAR request[32768],claimed[32768];path_for(request,L"lua-request.bin");path_for(claimed,L"lua-processing.bin");
 AcquireSRWLockExclusive(&queue_lock);
 if(pending){ReleaseSRWLockExclusive(&queue_lock);return;}
 if(!MoveFileExW(request,claimed,MOVEFILE_REPLACE_EXISTING)){ReleaseSRWLockExclusive(&queue_lock);return;}
 HANDLE f=CreateFileW(claimed,GENERIC_READ,FILE_SHARE_READ,NULL,OPEN_EXISTING,FILE_ATTRIBUTE_NORMAL,NULL);
 uint64_t id=0;DWORD size=0,read=0;char *script=NULL;
 if(f!=INVALID_HANDLE_VALUE) {
  DWORD total=GetFileSize(f,NULL);
  if(ReadFile(f,&id,8,&read,NULL)&&read==8 && ReadFile(f,&size,4,&read,NULL)&&read==4 && size && size<=MAX_SCRIPT && total==size+12) {
   script=HeapAlloc(GetProcessHeap(),0,size+1);
   if(script && (!ReadFile(f,script,size,&read,NULL)||read!=size)){HeapFree(GetProcessHeap(),0,script);script=NULL;}
  }
  CloseHandle(f);
 }
 DeleteFileW(claimed);
 if(script){script[size]=0;pending=script;pending_id=id;pending_size=size;status(id,"queued","Waiting for the next game main-thread Lua call");}
 else status(id,"error","Invalid Lua request or insufficient memory");
 ReleaseSRWLockExclusive(&queue_lock);
}
static DWORD WINAPI worker(void *unused) {
 if(!GetModuleFileNameW(self_module,root,32768))return 1;
 WCHAR *separator=wcsrchr(root,L'\\');if(!separator)return 1;*separator=0;
 depth_slot=TlsAlloc();if(depth_slot==TLS_OUT_OF_INDEXES){status(0,"error","TLS allocation failed");return 1;}
 status(0,"waiting","Waiting for the game xLua runtime");
 HMODULE lua=NULL;
 for(unsigned i=0;i<2400 && !lua;i++){lua=find_xlua();if(!lua)Sleep(250);}
 if(!lua){status(0,"error","Game xLua runtime not found after 10 minutes");return 1;}
 loadbuffer=(LoadBuffer)GetProcAddress(lua,"xluaL_loadbuffer");
 gettop=(GetTop)GetProcAddress(lua,"lua_gettop");settop=(SetTop)GetProcAddress(lua,"lua_settop");tostring=(ToString)GetProcAddress(lua,"lua_tolstring");
 void *target=(void *)GetProcAddress(lua,"lua_pcall");
 if(!loadbuffer||!gettop||!settop||!tostring||!target){status(0,"error","Unsupported game xLua exports");return 1;}
 MH_STATUS err=MH_Initialize();
 if(err==MH_OK)err=MH_CreateHook(target,(void *)hook_pcall,(void **)&original_pcall);
 if(err==MH_OK)err=MH_EnableHook(target);
 if(err!=MH_OK){status(0,"error",MH_StatusToString(err));return 1;}
 status(0,"ready","Enter the game, then execute selected scripts");
 for(;;){EnumWindows(find_window,0);read_request();Sleep(200);}
}
BOOL WINAPI DllMain(HINSTANCE module,DWORD reason,LPVOID reserved) {
 if(reason==DLL_PROCESS_ATTACH){self_module=module;DisableThreadLibraryCalls(module);HANDLE t=CreateThread(NULL,0,worker,NULL,0,NULL);if(t)CloseHandle(t);}
 return TRUE;
}
