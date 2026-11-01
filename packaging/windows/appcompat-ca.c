/* Native x64, per-user MSI custom actions. Link with msi and advapi32.
 *
 * PrepareAppCompat is immediate: run AFTER RemoveExistingProducts, once the
 * formatted AppCompatPath property is [INSTALLDIR]agent-sessions.exe. Set
 * AppCompatRemove to exactly "1" for uninstall, or empty for install/repair.
 * Schedule RollbackAppCompat (rollback) BEFORE ApplyAppCompat (deferred), both
 * impersonated, after PrepareAppCompat. Their action IDs must match these names:
 * Prepare sets those properties to the encoded CustomActionData snapshot.
 * Do not schedule these actions in the UI sequence or with Impersonate="no".
 *
 * APPCOMPAT_REGISTRY_KEY is a compile-time-only test seam. There is deliberately
 * no MSI property/environment variable for redirecting production registry I/O.
 * APPCOMPAT_CORE_TEST builds the platform-independent tests by including this
 * source, without any Windows headers or entry points.
 */
#include <stdint.h>
#include <stdlib.h>
#include <string.h>
#include <wchar.h>
#include <limits.h>

#ifndef APPCOMPAT_CORE_TEST
#ifndef _WIN64
#error Build this custom action as a native Windows x64 DLL.
#endif
#define WIN32_LEAN_AND_MEAN
#include <windows.h>
#include <msi.h>
#include <msiquery.h>
#ifndef APPCOMPAT_REGISTRY_KEY
#define APPCOMPAT_REGISTRY_KEY L"Software\\Microsoft\\Windows NT\\CurrentVersion\\AppCompatFlags\\Layers"
#endif
#endif

#define AC_OK 0UL
#define AC_INVALID 13UL /* ERROR_INVALID_DATA */
#define AC_NOMEM 8UL   /* ERROR_NOT_ENOUGH_MEMORY */
#define AC_TOKEN L"DISABLEDXMAXIMIZEDWINDOWEDMODE"

/* The snapshot is raw registry bytes, not a normalized string. This preserves
 * missing terminators and extra trailing NULs byte-for-byte on rollback. */
typedef struct {
    wchar_t *path;
    unsigned char *bytes;
    size_t size;
    int exists;
    int remove;
} Snapshot;

static void snapshot_free(Snapshot *s)
{
    free(s->path);
    free(s->bytes);
    memset(s, 0, sizeof(*s));
}

static int space(wchar_t c)
{
    return c == L' ' || c == L'\t' || c == L'\r' || c == L'\n' ||
           c == L'\v' || c == L'\f';
}

static wchar_t ascii_upper(wchar_t c)
{
    return c >= L'a' && c <= L'z' ? c - (L'a' - L'A') : c;
}

static int target_token(const wchar_t *s, size_t n)
{
    size_t i;
    if (n != sizeof(AC_TOKEN) / sizeof(wchar_t) - 1)
        return 0;
    for (i = 0; i < n; ++i)
        if (ascii_upper(s[i]) != AC_TOKEN[i])
            return 0;
    return 1;
}

/* Convert registry UTF-16LE to text without depending on host wchar_t width.
 * Reject hidden text after an embedded NUL and odd byte counts rather than
 * silently destroying malformed data. An unterminated REG_SZ is readable. */
static unsigned long registry_text(const Snapshot *s, wchar_t **text)
{
    size_t i, n = s->size / 2;
    int ended = 0;
    wchar_t *p;
    if (s->size % 2 || n > SIZE_MAX / sizeof(*p) - 1)
        return AC_INVALID;
    p = calloc(n + 1, sizeof(*p));
    if (!p)
        return AC_NOMEM;
    for (i = 0; i < n; ++i) {
        unsigned int c = s->bytes[2*i] | ((unsigned int)s->bytes[2*i+1] << 8);
        if (!c)
            ended = 1;
        else if (ended) {
            free(p);
            return AC_INVALID;
        }
        p[i] = (wchar_t)c;
    }
    *text = p;
    return AC_OK;
}

/* Strip only complete target tokens, retaining every other token (including
 * compatibility markers). Whitespace is preserved on no-op; when editing it
 * is normalized to single spaces. A bare '~' is not a compatibility setting:
 * remove that otherwise empty value on uninstall, but retain other markers. */
static unsigned long layer_value(const Snapshot *s, wchar_t **result, int *present)
{
    wchar_t *text = NULL, *out;
    size_t i = 0, used = 0, start, n, target_count = 0;
    int substantive = 0, has_tilde = 0;
    unsigned long err = registry_text(s, &text);
    if (err)
        return err;
    n = wcslen(text);
    if (n > SIZE_MAX / sizeof(*out) - (sizeof(AC_TOKEN) / sizeof(wchar_t)) - 4) {
        free(text);
        return AC_NOMEM;
    }
    out = calloc(n + sizeof(AC_TOKEN) / sizeof(wchar_t) + 4, sizeof(*out));
    if (!out) {
        free(text);
        return AC_NOMEM;
    }
    while (i < n) {
        while (i < n && space(text[i])) ++i;
        start = i;
        while (i < n && !space(text[i])) ++i;
        if (i == start) break;
        if (target_token(text + start, i - start)) {
            ++target_count;
            continue;
        }
        if (i - start == 1 && text[start] == L'~') has_tilde = 1;
        else substantive = 1;
        if (used) out[used++] = L' ';
        memcpy(out + used, text + start, (i - start) * sizeof(*out));
        used += i - start;
    }
    if (!s->remove) {
        /* Preserve existing semantics: add '~' only to an empty value. */
        if (!used && !has_tilde) out[used++] = L'~';
        if (used) out[used++] = L' ';
        memcpy(out + used, AC_TOKEN, sizeof(AC_TOKEN));
        *present = 1;
    } else {
        *present = substantive;
    }
    /* Avoid gratuitous whitespace/case changes when nothing needs changing. */
    if ((s->remove && target_count == 0) || (!s->remove && target_count == 1)) {
        free(out);
        out = text;
        text = NULL;
        *present = s->exists;
    }
    free(text);
    *result = out;
    return AC_OK;
}

static int unhex(wchar_t c)
{
    if (c >= L'0' && c <= L'9') return c - L'0';
    if (c >= L'A' && c <= L'F') return c - L'A' + 10;
    return -1;
}

/* ASCII-only, versioned envelope. Neither path delimiters nor registry text
 * can escape their fields. Path is UTF-16 code units, snapshot is raw bytes.
 * Format: 1|<remove 0/1>|<exists 0/1>|<path hex4>|<snapshot hex2> */
static unsigned long snapshot_encode(const Snapshot *s, wchar_t **encoded)
{
    static const wchar_t hex[] = L"0123456789ABCDEF";
    size_t n = wcslen(s->path), len, i, j = 6;
    wchar_t *p;
    if (!n || (!s->exists && s->size) || (s->remove != 0 && s->remove != 1) ||
        (s->exists != 0 && s->exists != 1)) return AC_INVALID;
    if (n > (SIZE_MAX - 8) / 4 || s->size > (SIZE_MAX - 8 - n*4) / 2)
        return AC_NOMEM;
    len = 8 + n*4 + s->size*2;
    if (len > SIZE_MAX / sizeof(*p)) return AC_NOMEM;
    p = malloc(len * sizeof(*p));
    if (!p) return AC_NOMEM;
    p[0] = L'1'; p[1] = L'|'; p[2] = L'0' + s->remove;
    p[3] = L'|'; p[4] = L'0' + s->exists; p[5] = L'|';
    for (i = 0; i < n; ++i) {
        unsigned long c = (unsigned long)s->path[i];
        if (c > 0xffff) { free(p); return AC_INVALID; }
        p[j++] = hex[(c >> 12) & 15]; p[j++] = hex[(c >> 8) & 15];
        p[j++] = hex[(c >> 4) & 15]; p[j++] = hex[c & 15];
    }
    p[j++] = L'|';
    for (i = 0; i < s->size; ++i) {
        p[j++] = hex[s->bytes[i] >> 4]; p[j++] = hex[s->bytes[i] & 15];
    }
    p[j] = 0;
    *encoded = p;
    return AC_OK;
}

static unsigned long snapshot_decode(const wchar_t *p, Snapshot *s)
{
    const wchar_t *sep;
    size_t len = wcslen(p), n, b, i, j;
    memset(s, 0, sizeof(*s));
    if (len < 11 || p[0] != L'1' || p[1] != L'|' ||
        (p[2] != L'0' && p[2] != L'1') || p[3] != L'|' ||
        (p[4] != L'0' && p[4] != L'1') || p[5] != L'|') return AC_INVALID;
    sep = wcschr(p + 6, L'|');
    if (!sep || sep == p + 6 || (sep - (p + 6)) % 4) return AC_INVALID;
    n = (size_t)(sep - (p + 6)) / 4;
    b = wcslen(sep + 1);
    if (b % 2 || (p[4] == L'0' && b)) return AC_INVALID;
    if (n > SIZE_MAX / sizeof(*s->path) - 1) return AC_NOMEM;
    s->path = calloc(n + 1, sizeof(*s->path));
    s->bytes = malloc(b / 2 + 1);
    if (!s->path || !s->bytes) { snapshot_free(s); return AC_NOMEM; }
    for (i = 0; i < n; ++i) {
        unsigned int c = 0;
        for (j = 0; j < 4; ++j) {
            int v = unhex(p[6 + i*4 + j]);
            if (v < 0) goto invalid;
            c = (c << 4) | (unsigned int)v;
        }
        if (!c) goto invalid;
        s->path[i] = (wchar_t)c;
    }
    for (i = 0; i < b/2; ++i) {
        int hi = unhex(sep[1 + i*2]), lo = unhex(sep[2 + i*2]);
        if (hi < 0 || lo < 0) goto invalid;
        s->bytes[i] = (unsigned char)((hi << 4) | lo);
    }
    s->size = b/2; s->exists = p[4] - L'0'; s->remove = p[2] - L'0';
    return AC_OK;
invalid:
    snapshot_free(s);
    return AC_INVALID;
}

#ifndef APPCOMPAT_CORE_TEST
static UINT report(MSIHANDLE install, const wchar_t *operation, unsigned long err)
{
    MSIHANDLE record = MsiCreateRecord(2);
    if (record) {
        MsiRecordSetStringW(record, 0, L"AppCompat: [1] failed (error [2]).");
        MsiRecordSetStringW(record, 1, operation);
        MsiRecordSetInteger(record, 2, (int)err);
        MsiProcessMessage(install, INSTALLMESSAGE_ERROR, record);
        MsiCloseHandle(record);
    }
    return ERROR_INSTALL_FAILURE;
}

static unsigned long property(MSIHANDLE install, const wchar_t *name, wchar_t **value)
{
    DWORD capacity = 1;
    wchar_t *p = NULL;
    for (;;) {
        DWORD count = capacity;
        UINT err;
        wchar_t *next;
        /* DWORD-sized capacities fit size_t * sizeof(wchar_t) on x64. */
        next = realloc(p, (size_t)capacity * sizeof(*p));
        if (!next) { free(p); return AC_NOMEM; }
        p = next;
        err = MsiGetPropertyW(install, name, p, &count);
        if (err == ERROR_SUCCESS) { *value = p; return AC_OK; }
        if (err != ERROR_MORE_DATA) { free(p); return err; }
        if (count == MAXDWORD) { free(p); return AC_NOMEM; }
        capacity = count + 1;
    }
}

static unsigned long read_snapshot(Snapshot *s)
{
    HKEY key;
    LSTATUS err = RegOpenKeyExW(HKEY_CURRENT_USER, APPCOMPAT_REGISTRY_KEY, 0,
                               KEY_QUERY_VALUE | KEY_WOW64_64KEY, &key);
    DWORD capacity = 0;
    if (err == ERROR_FILE_NOT_FOUND || err == ERROR_PATH_NOT_FOUND) return AC_OK;
    if (err) return (unsigned long)err;
    for (;;) {
        DWORD size = capacity, type = 0;
        err = RegQueryValueExW(key, s->path, NULL, &type, s->bytes, &size);
        if (err == ERROR_FILE_NOT_FOUND) { err = ERROR_SUCCESS; break; }
        if (err != ERROR_SUCCESS && err != ERROR_MORE_DATA) break;
        if (type != REG_SZ) { err = ERROR_DATATYPE_MISMATCH; break; }
        if (err == ERROR_SUCCESS && s->bytes) {
            s->size = size; s->exists = 1; break;
        }
        /* Retry the read if another process grew the value between queries. */
        {
            unsigned char *next = realloc(s->bytes, size ? (size_t)size : 1);
            if (!next) { err = ERROR_NOT_ENOUGH_MEMORY; break; }
            s->bytes = next; capacity = size;
        }
    }
    RegCloseKey(key);
    return (unsigned long)err;
}

static unsigned long write_value(const wchar_t *path, int present,
                                 const unsigned char *bytes, size_t size)
{
    HKEY key;
    LSTATUS err;
    if (size > MAXDWORD) return ERROR_INVALID_DATA;
    if (present)
        err = RegCreateKeyExW(HKEY_CURRENT_USER, APPCOMPAT_REGISTRY_KEY, 0, NULL,
                              REG_OPTION_NON_VOLATILE, KEY_SET_VALUE | KEY_WOW64_64KEY,
                              NULL, &key, NULL);
    else
        err = RegOpenKeyExW(HKEY_CURRENT_USER, APPCOMPAT_REGISTRY_KEY, 0,
                            KEY_SET_VALUE | KEY_WOW64_64KEY, &key);
    if (!present && (err == ERROR_FILE_NOT_FOUND || err == ERROR_PATH_NOT_FOUND))
        return AC_OK;
    if (err) return (unsigned long)err;
    if (present) err = RegSetValueExW(key, path, 0, REG_SZ, bytes, (DWORD)size);
    else err = RegDeleteValueW(key, path);
    RegCloseKey(key);
    if (!present && err == ERROR_FILE_NOT_FOUND) return AC_OK;
    return (unsigned long)err;
}

__declspec(dllexport) UINT WINAPI PrepareAppCompat(MSIHANDLE install)
{
    Snapshot s = {0};
    wchar_t *remove = NULL, *encoded = NULL, *check = NULL;
    int present;
    unsigned long err = property(install, L"AppCompatPath", &s.path);
    if (!err && !*s.path) err = AC_INVALID;
    if (!err) err = property(install, L"AppCompatRemove", &remove);
    if (!err && *remove && wcscmp(remove, L"1")) err = AC_INVALID;
    if (!err) {
        s.remove = *remove == L'1';
        err = read_snapshot(&s);
    }
    /* Validate transformation before registering a rollback action. */
    if (!err) err = layer_value(&s, &check, &present);
    if (!err) err = snapshot_encode(&s, &encoded);
    if (!err) err = MsiSetPropertyW(install, L"RollbackAppCompat", encoded);
    if (!err) err = MsiSetPropertyW(install, L"ApplyAppCompat", encoded);
    free(remove); free(encoded); free(check); snapshot_free(&s);
    return err ? report(install, L"prepare", err) : ERROR_SUCCESS;
}

static int same_value(const Snapshot *a, const Snapshot *b)
{
    return a->exists == b->exists && (!a->exists ||
           (a->size == b->size && (!a->size || !memcmp(a->bytes, b->bytes, a->size))));
}

static UINT execute(MSIHANDLE install, int rollback)
{
    Snapshot s = {0}, expected = {0}, live = {0};
    wchar_t *data = NULL, *text = NULL, *original = NULL;
    int present = 0;
    unsigned long err = property(install, L"CustomActionData", &data);
    if (!err) err = snapshot_decode(data, &s);
    if (!err) err = layer_value(&s, &text, &present);
    if (!err) err = registry_text(&s, &original);
    if (!err) {
        expected = s; /* borrowed storage; default is an exact raw no-op */
        if (!(present == s.exists && !wcscmp(text, original))) {
            size_t n = wcslen(text);
            if (n >= MAXDWORD / sizeof(wchar_t)) err = AC_INVALID;
            else {
                expected.exists = present;
                expected.bytes = (unsigned char *)text;
                expected.size = present ? (n + 1) * sizeof(wchar_t) : 0;
            }
        }
    }
    if (!err) {
        live.path = s.path; /* borrowed; only live.bytes is allocated */
        err = read_snapshot(&live);
    }
    if (!err) {
        /* Do not overwrite changes made since Prepare, or user edits made
         * after Apply. Registry has no compare-and-swap API: this guards the
         * deferred-action gap, not a simultaneous writer during this call. */
        if (rollback) {
            if (!same_value(&live, &s)) {
                if (!same_value(&live, &expected)) err = ERROR_REVISION_MISMATCH;
                else err = write_value(s.path, s.exists, s.bytes, s.size);
            }
        } else if (!same_value(&live, &s)) err = ERROR_REVISION_MISMATCH;
        else if (!same_value(&s, &expected))
            err = write_value(s.path, expected.exists, expected.bytes, expected.size);
    }
    free(live.bytes); free(data); free(text); free(original); snapshot_free(&s);
    return err ? report(install, rollback ? L"rollback" : L"apply", err) : ERROR_SUCCESS;
}

__declspec(dllexport) UINT WINAPI ApplyAppCompat(MSIHANDLE install)
{
    return execute(install, 0);
}

__declspec(dllexport) UINT WINAPI RollbackAppCompat(MSIHANDLE install)
{
    return execute(install, 1);
}
#endif
