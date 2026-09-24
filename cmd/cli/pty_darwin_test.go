package main

import (
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

// unlockPTY grants and unlocks the pseudo-terminal behind master and returns
// the path of its terminal side, which shares the master's minor number.
func unlockPTY(master *os.File) (string, error) {
	fd := int(master.Fd())
	if err := unix.IoctlSetInt(fd, unix.TIOCPTYGRANT, 0); err != nil {
		return "", err
	}
	if err := unix.IoctlSetInt(fd, unix.TIOCPTYUNLK, 0); err != nil {
		return "", err
	}
	var st unix.Stat_t
	if err := unix.Fstat(fd, &st); err != nil {
		return "", err
	}
	return fmt.Sprintf("/dev/ttys%03d", unix.Minor(uint64(st.Rdev))), nil
}
