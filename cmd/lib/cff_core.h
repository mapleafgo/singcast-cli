#ifndef CFF_CORE_H
#define CFF_CORE_H

#include <stdint.h>

// Event type constants passed to the CoreCallback function.
#define CFF_EVENT_TRAFFIC      0
#define CFF_EVENT_LOGS         1
#define CFF_EVENT_CONNECTIONS  2
#define CFF_EVENT_PROXY_UPDATE 3
#define CFF_EVENT_MODE_UPDATE  4
#define CFF_EVENT_CORE_LOG     6

// Log level constants.
#define CFF_LEVEL_ERROR 2
#define CFF_LEVEL_WARN  3
#define CFF_LEVEL_INFO  4
#define CFF_LEVEL_DEBUG 5
#define CFF_LEVEL_TRACE 6

// Callback type: void callback(int eventType, const char* jsonPayload)
typedef void (*CoreCallback)(int, const char*);

// --- Lifecycle ---
extern char* CoreInit(const char* homeDir);
extern char* CoreStartWithContent(const char* content, const char* ruleSetProxy);
extern char* CoreStop();
extern void  CoreDestroy();

// --- Config ---
extern char* CoreCheckConfig(const char* content);
extern char* CoreReloadConfig(const char* content, const char* ruleSetProxy);
extern char* CoreReloadTUN();

// --- Pause / Wake / Network ---
extern void CorePause();
extern void CoreWake();
extern void CoreResetNetwork();

// --- Proxy Control ---
extern char* CoreSelectProxy(const char* group, const char* tag);
extern char* CoreTestDelay(const char* name);
extern char* CoreSetMode(const char* mode);
extern char* CoreSetGroupExpand(const char* group, int expand);

// --- Queries ---
extern char* CoreQueryProxies();
extern char* CoreQueryTraffic();
extern char* CoreQueryLogs();
extern char* CoreQueryConnections();
extern char* CoreClearLogs();
extern char* CoreGetStartedAt();

// --- Connection Management ---
extern char* CoreCloseConnection(const char* id);
extern char* CoreCloseAllConnections();

// --- Platform ---
extern int  CoreNeedWIFIState();
extern int  CoreNeedFindProcess();
extern void CoreUpdateWIFIState();
extern void CoreSetIncludeAllNetworks(int v);
extern void CoreWriteMessage(int level, const char* message);

// --- Memory ---
extern void CoreForceGC();
extern char* CoreSetMemoryLimit(const char* bytes);
extern char* CoreQueryMemoryStats();
extern void CoreFlushSystemDNS();

// --- Version ---
extern char* CoreGetVersion();

// --- Events ---
extern void CoreSetCallback(void* cb);

// --- Utilities ---
extern char* CoreFormatBytes(int64_t length);
extern char* CoreFormatDuration(int64_t duration);
extern char* CoreAvailablePort(int startPort);

// --- Memory Management ---
extern void CoreFreeString(char* s);

#endif // CFF_CORE_H
