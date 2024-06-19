//go:build !windows
// +build !windows

package prompt

import (
	"syscall"

	"github.com/RainyBow/go-prompt/internal/term"
	"golang.org/x/sys/unix"
)

const maxReadBytes = 1024
const defaultRow = 50
const defaultCol = 100

// PosixParser is a ConsoleParser implementation for POSIX environment.
type PosixParser struct {
	fd          int
	origTermios syscall.Termios
}

// Setup should be called before starting input
func (t *PosixParser) Setup() error {
	// Set NonBlocking mode because if syscall.Read block this goroutine, it cannot receive data from stopCh.
	if err := syscall.SetNonblock(t.fd, true); err != nil {
		return err
	}
	if err := term.SetRaw(t.fd); err != nil {
		return err
	}
	return nil
}

// TearDown should be called after stopping input
func (t *PosixParser) TearDown() error {
	if err := syscall.SetNonblock(t.fd, false); err != nil {
		return err
	}
	if err := term.Restore(); err != nil {
		return err
	}
	return nil
}
func (t *PosixParser) TearDownDisableEcho() error { // 关闭回显
	if err := syscall.SetNonblock(t.fd, false); err != nil {
		return err
	}
	if err := term.RestoreDisableEcho(); err != nil {
		return err
	}
	return nil
}

// Read returns byte array.
func (t *PosixParser) Read() ([]byte, error) {
	buf := make([]byte, maxReadBytes)
	n, err := syscall.Read(t.fd, buf)
	if err != nil {
		return []byte{}, err
	}
	return buf[:n], nil
}

// GetWinSize returns WinSize object to represent width and height of terminal.
func (t *PosixParser) GetWinSize() *WinSize {
	ws, err := unix.IoctlGetWinsize(t.fd, unix.TIOCGWINSZ)
	if err != nil {
		panic(err)
	}
	return &WinSize{
		Row: ws.Row,
		Col: ws.Col,
	}
}
func (t *PosixParser) SetWinSize(winsize *WinSize) {
	err := unix.IoctlSetWinsize(t.fd, unix.TIOCSWINSZ, &unix.Winsize{
		Row: winsize.Row,
		Col: winsize.Col,
	})
	if err != nil {
		panic(err)
	}
}

var _ ConsoleParser = &PosixParser{}

// NewStandardInputParser returns ConsoleParser object to read from stdin.
func NewStandardInputParser() *PosixParser {
	in, err := syscall.Open("/dev/tty", syscall.O_RDONLY, 0)
	if err != nil {
		panic(err)
	}

	return &PosixParser{
		fd: in,
	}
}

// just for console(serial) which window size is 0
func NewConsoleInputParser(winsize WinSize) *PosixParser {
	in, err := syscall.Open("/dev/ttyS0", syscall.O_RDONLY, 0)
	if err != nil {
		in, err = syscall.Open("/dev/console", syscall.O_RDONLY, 0)
		if err != nil {
			panic(err)
		}
	}

	ret := &PosixParser{
		fd: in,
	}
	tmp_size := ret.GetWinSize()
	if tmp_size.Row == 0 || tmp_size.Col == 0 {
		ret.SetWinSize(&winsize)
	}
	return ret
}
