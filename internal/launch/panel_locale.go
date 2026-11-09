package launch

// panelRemoteLocale gives an unconfigured remote shell a locale that can read
// the terminal's UTF-8 input. Desktop-launched SSH may have no LANG to forward.
// Discover names remotely: a locale available on the client may not exist there.
// Explicit settings (including C/POSIX) belong to the user and remain untouched.
const panelRemoteLocale = `if [ -z "${LC_ALL:-}" ] && [ -z "${LC_CTYPE:-}" ] && [ -z "${LANG:-}" ]; then
  __as_locales=$(LC_ALL=C locale -a 2>/dev/null) || __as_locales=
  __as_utf8=
  __as_english_utf8=
  while IFS= read -r __as_locale; do
    case "$__as_locale" in
      [Cc].[Uu][Tt][Ff]8|[Cc].[Uu][Tt][Ff]-8)
        __as_utf8=$__as_locale
        break
        ;;
      *.[Uu][Tt][Ff]8|*.[Uu][Tt][Ff]-8)
        [ -n "$__as_utf8" ] || __as_utf8=$__as_locale
        case "$__as_locale" in en_US.*) __as_english_utf8=$__as_locale;; esac
        ;;
    esac
  done <<AGENT_SESSIONS_LOCALES
$__as_locales
AGENT_SESSIONS_LOCALES
  case "$__as_utf8" in [Cc].*) ;; *) __as_utf8=${__as_english_utf8:-$__as_utf8};; esac
  if [ -n "$__as_utf8" ]; then
    LANG=$__as_utf8
    export LANG
  fi
  unset __as_locales __as_utf8 __as_english_utf8 __as_locale
fi
`
