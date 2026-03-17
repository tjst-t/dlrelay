//go:build windows

package server

type diskStat struct {
	free  int64
	total int64
}

func diskUsage(path string) (diskStat, error) {
	return diskStat{free: -1, total: -1}, nil
}
