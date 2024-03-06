package libFastHttpImpl

import (
	"os"
	"syscall"
)

func getUpdateDate(fi os.FileInfo) syscall.Timespec {
	return fi.Sys().(*syscall.Stat_t).Mtimespec
}
