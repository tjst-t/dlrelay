//go:build linux || darwin || freebsd

package server

import "syscall"

type diskStat struct {
	free  int64
	total int64
}

func diskUsage(path string) (diskStat, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return diskStat{}, err
	}
	return diskStat{
		free:  int64(stat.Bavail) * int64(stat.Bsize),
		total: int64(stat.Blocks) * int64(stat.Bsize),
	}, nil
}
