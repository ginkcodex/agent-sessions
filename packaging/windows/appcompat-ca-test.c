/* gcc -std=c11 -Wall -Wextra -Werror -pedantic -fsanitize=address,undefined
 *     packaging/windows/appcompat-ca-test.c -o /tmp/appcompat-ca-test
 * /tmp/appcompat-ca-test
 * No Windows SDK needed; exercises the exact production tokenizer/codec.
 */
#define APPCOMPAT_CORE_TEST
#include "appcompat-ca.c"
#include <assert.h>
#include <stdio.h>

static Snapshot snapshot(const wchar_t *text, int exists, int remove)
{
    Snapshot s = {0};
    size_t i, n = wcslen(text);
    s.path = malloc(128 * sizeof(wchar_t));
    assert(s.path);
    wcscpy(s.path, L"C:\\Users\\Name | ; = \\Agent Sessions\\\x4e2d\xd83d\xde00.exe");
    s.exists = exists;
    s.remove = remove;
    s.size = exists ? (n + 1) * 2 : 0;
    s.bytes = calloc(s.size + 1, 1);
    assert(s.bytes);
    for (i = 0; i < n && exists; ++i) {
        s.bytes[i*2] = (unsigned char)text[i];
        s.bytes[i*2+1] = (unsigned char)((unsigned int)text[i] >> 8);
    }
    return s;
}

static void layers(const wchar_t *before, int exists, int remove,
                   const wchar_t *after, int present)
{
    Snapshot s = snapshot(before, exists, remove);
    wchar_t *out = NULL;
    int actual = -1;
    assert(layer_value(&s, &out, &actual) == AC_OK);
    assert(actual == present);
    if (present) assert(wcscmp(out, after) == 0);
    free(out);
    snapshot_free(&s);
}

static void roundtrip(Snapshot *s)
{
    wchar_t *encoded = NULL;
    Snapshot restored;
    assert(snapshot_encode(s, &encoded) == AC_OK);
    assert(snapshot_decode(encoded, &restored) == AC_OK);
    assert(wcscmp(s->path, restored.path) == 0);
    assert(s->size == restored.size);
    assert(s->exists == restored.exists && s->remove == restored.remove);
    assert(memcmp(s->bytes, restored.bytes, s->size) == 0);
    snapshot_free(&restored);
    free(encoded);
}

int main(void)
{
    Snapshot s, decoded;
    wchar_t *out = NULL;
    int present;
    size_t i;
    const wchar_t *invalid[] = {
        L"", L"2|0|0|0041|", L"1|2|0|0041|", L"1|0|2|0041|",
        L"1|0|0||", L"1|0|0|041|", L"1|0|0|0000|",
        L"1|0|0|0041|00", L"1|0|1|0041|0", L"1|0|1|0041|GG",
        L"1|0|1|00X1|", L"1|0|1|0041||", L"1|0|1|00410000|"
    };
    layers(L"", 0, 0, L"~ " AC_TOKEN, 1);
    layers(L"", 1, 0, L"~ " AC_TOKEN, 1);
    layers(L"~", 1, 0, L"~ " AC_TOKEN, 1);
    layers(L"~ HIGHDPIAWARE WIN7RTM", 1, 0,
           L"~ HIGHDPIAWARE WIN7RTM " AC_TOKEN, 1);
    layers(L"HIGHDPIAWARE", 1, 0, L"HIGHDPIAWARE " AC_TOKEN, 1);
    layers(L"  ~\t" AC_TOKEN L"  WIN7RTM  ", 1, 0,
           L"  ~\t" AC_TOKEN L"  WIN7RTM  ", 1);
    layers(L"~ disabledxmaximizedwindowedmode", 1, 0,
           L"~ disabledxmaximizedwindowedmode", 1);
    layers(L"~ " AC_TOKEN L" " AC_TOKEN L" WIN7RTM", 1, 0,
           L"~ WIN7RTM " AC_TOKEN, 1);
    layers(L"~ " AC_TOKEN L" disabledxmaximizedwindowedmode", 1, 1, L"", 0);
    layers(L"~ " AC_TOKEN, 1, 1, L"", 0);
    layers(AC_TOKEN, 1, 1, L"", 0);
    layers(L"~ " AC_TOKEN L" WIN7RTM", 1, 1, L"~ WIN7RTM", 1);
    layers(L"# ~ " AC_TOKEN, 1, 1, L"# ~", 1);
    layers(L"~ " AC_TOKEN L"X X" AC_TOKEN, 1, 1,
           L"~ " AC_TOKEN L"X X" AC_TOKEN, 1);
    layers(L"\t~  WIN7RTM  ", 1, 1, L"\t~  WIN7RTM  ", 1);
    layers(L"~", 1, 1, L"~", 1); /* not ours: no target token */
    layers(L"", 0, 1, L"", 0);
    layers(L"", 1, 1, L"", 1);

    for (i = 0; i < sizeof(invalid)/sizeof(invalid[0]); ++i)
        assert(snapshot_decode(invalid[i], &decoded) == AC_INVALID);
    s = snapshot(L"~  WIN7RTM\t" AC_TOKEN, 1, 0);
    roundtrip(&s);
    s.remove = 1;
    roundtrip(&s);
    /* Exact nonterminated and extra-NUL snapshots survive encoding. */
    s.size -= 2;
    roundtrip(&s);
    s.size += 2;
    s.bytes = realloc(s.bytes, s.size + 4);
    assert(s.bytes);
    memset(s.bytes + s.size, 0, 4);
    s.size += 4;
    roundtrip(&s);
    /* Odd raw byte payload is preserved by the codec but rejected for edits. */
    --s.size;
    roundtrip(&s);
    assert(layer_value(&s, &out, &present) == AC_INVALID);
    snapshot_free(&s);
    s = snapshot(L"~\0", 1, 0);
    s.bytes = realloc(s.bytes, 6);
    assert(s.bytes);
    s.size = 6;
    s.bytes[4] = 'X'; s.bytes[5] = 0;
    assert(layer_value(&s, &out, &present) == AC_INVALID);
    roundtrip(&s);
    snapshot_free(&s);
    s = snapshot(L"", 0, 1);
    roundtrip(&s);
    snapshot_free(&s);
    s = snapshot(L"", 1, 0);
    s.size = 0; /* zero-byte REG_SZ is not absence */
    roundtrip(&s);
    snapshot_free(&s);
    /* Exercise dynamic allocation beyond typical fixed registry buffers. */
    {
        size_t n = 100000;
        wchar_t *large = malloc((n + 1) * sizeof(*large));
        assert(large);
        for (i = 0; i < n; ++i) large[i] = L'X';
        large[n] = 0;
        s = snapshot(large, 1, 0);
        assert(layer_value(&s, &out, &present) == AC_OK && present);
        assert(wcslen(out) == n + 1 + wcslen(AC_TOKEN));
        free(s.path);
        s.path = malloc((n + 1) * sizeof(*s.path));
        assert(s.path);
        memcpy(s.path, large, (n + 1) * sizeof(*s.path));
        roundtrip(&s); /* path/property encoding is dynamic as well */
        free(out); free(large); snapshot_free(&s);
    }
    puts("AppCompat tokenizer and snapshot tests passed");
    return 0;
}
