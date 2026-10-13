package main

import (
	"os"
	"strconv"

	"golang.org/x/sys/unix"
)

// unlockPTY unlocks the pseudo-terminal behind master and returns the path of
// its terminal side.
func unlockPTY(master *os.File) (string, error) {
	fd := int(master.Fd())
	if err := unix.IoctlSetPointerInt(fd, unix.TIOCSPTLCK, 0); err != nil {
		return "", err
	}
	n, err := unix.IoctlGetUint32(fd, unix.TIOCGPTN)
	if err != nil {
		return "", err
	}
	return "/dev/pts/" + strconv.FormatUint(uint64(n), 10), nil
}
