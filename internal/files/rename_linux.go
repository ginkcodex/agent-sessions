package files

import (
	"os"

	"golang.org/x/sys/unix"
)

func renameNoReplace(parent *os.File, source, name string) error {
	return unix.Renameat2(int(parent.Fd()), source, int(parent.Fd()), name, unix.RENAME_NOREPLACE)
}
