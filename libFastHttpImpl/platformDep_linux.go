package libFastHttpImpl

func getUpdateDate(fi os.FileInfo) syscall.Timespec {
	return fi.Sys().(*syscall.Stat_t).Mtim
}
