//go:build darwin

package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"syscall"
	"time"
	"unicode/utf8"
	"unsafe"
)

type keyKind int

const (
	keyRune keyKind = iota
	keyPaste
	keyEnter
	keyEsc
	keyBackspace
	keyDelete
	keyUp
	keyDown
	keyLeft
	keyRight
	keyHome
	keyEnd
	keyCtrlA
	keyCtrlC
	keyCtrlE
	keyCtrlU
	keyCtrlW
)

type keyEvent struct {
	kind keyKind
	text string
}

type terminal struct {
	in       *os.File
	out      *os.File
	fd       int
	original syscall.Termios
	closed   bool
}

func openTerminal() (*terminal, error) {
	fd := int(os.Stdin.Fd())

	var original syscall.Termios
	if err := ioctl(fd, syscall.TIOCGETA, unsafe.Pointer(&original)); err != nil {
		return nil, fmt.Errorf("stdin is not a terminal: %w", err)
	}

	raw := original
	raw.Iflag &^= syscall.IGNBRK | syscall.BRKINT | syscall.PARMRK |
		syscall.ISTRIP | syscall.INLCR | syscall.IGNCR | syscall.ICRNL | syscall.IXON
	raw.Oflag &^= syscall.OPOST
	raw.Lflag &^= syscall.ECHO | syscall.ECHONL | syscall.ICANON | syscall.ISIG | syscall.IEXTEN
	raw.Cflag &^= syscall.CSIZE | syscall.PARENB
	raw.Cflag |= syscall.CS8
	raw.Cc[syscall.VMIN] = 1
	raw.Cc[syscall.VTIME] = 0

	if err := ioctl(fd, syscall.TIOCSETA, unsafe.Pointer(&raw)); err != nil {
		return nil, fmt.Errorf("enter raw mode: %w", err)
	}

	t := &terminal{
		in:       os.Stdin,
		out:      os.Stdout,
		fd:       fd,
		original: original,
	}

	if err := t.write("\x1b[?1049h\x1b[?25l\x1b[?2004h\x1b[?7l\x1b[2J\x1b[H"); err != nil {
		_ = t.restore()
		return nil, err
	}
	return t, nil
}

func (t *terminal) Close() error {
	if t.closed {
		return nil
	}
	t.closed = true

	writeErr := t.write("\x1b[0m\x1b[?7h\x1b[?2004l\x1b[?25h\x1b[?1049l")
	restoreErr := t.restore()
	if writeErr != nil {
		return writeErr
	}
	return restoreErr
}

func (t *terminal) restore() error {
	if err := ioctl(t.fd, syscall.TIOCSETA, unsafe.Pointer(&t.original)); err != nil {
		return fmt.Errorf("restore terminal: %w", err)
	}
	return nil
}

func (t *terminal) write(s string) error {
	if _, err := io.WriteString(t.out, s); err != nil {
		return fmt.Errorf("write terminal: %w", err)
	}
	return nil
}

func (t *terminal) size() (width, height int) {
	var ws struct {
		rows   uint16
		cols   uint16
		xpixel uint16
		ypixel uint16
	}
	if err := ioctl(t.fd, syscall.TIOCGWINSZ, unsafe.Pointer(&ws)); err != nil {
		return 80, 24
	}
	width, height = int(ws.cols), int(ws.rows)
	if width <= 0 {
		width = 80
	}
	if height <= 0 {
		height = 24
	}
	return width, height
}

func ioctl(fd int, request uintptr, arg unsafe.Pointer) error {
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), request, uintptr(arg))
	if errno != 0 {
		return errno
	}
	return nil
}

func readKeys(f *os.File, out chan<- keyEvent) {
	defer close(out)

	reader := bufio.NewReaderSize(f, 256)
	fd := int(f.Fd())

	for {
		b, err := reader.ReadByte()
		if err != nil {
			return
		}

		event, ok := decodeByte(reader, fd, b)
		if ok {
			out <- event
		}
	}
}

func decodeByte(r *bufio.Reader, fd int, b byte) (keyEvent, bool) {
	switch {
	case b == 0x1b:
		return decodeEscape(r, fd), true
	case b == '\r' || b == '\n':
		return keyEvent{kind: keyEnter}, true
	case b == 0x7f || b == 0x08:
		return keyEvent{kind: keyBackspace}, true
	case b == 0x01:
		return keyEvent{kind: keyCtrlA}, true
	case b == 0x03:
		return keyEvent{kind: keyCtrlC}, true
	case b == 0x05:
		return keyEvent{kind: keyCtrlE}, true
	case b == 0x15:
		return keyEvent{kind: keyCtrlU}, true
	case b == 0x17:
		return keyEvent{kind: keyCtrlW}, true
	case b < 0x20:
		return keyEvent{}, false
	case b < utf8.RuneSelf:
		return keyEvent{kind: keyRune, text: string(b)}, true
	}

	runeValue, ok := readRune(r, b)
	if !ok {
		return keyEvent{}, false
	}
	return keyEvent{kind: keyRune, text: string(runeValue)}, true
}

func decodeEscape(r *bufio.Reader, fd int) keyEvent {
	if r.Buffered() == 0 && !waitReadable(fd, 25*time.Millisecond) {
		return keyEvent{kind: keyEsc}
	}

	b, err := r.ReadByte()
	if err != nil {
		return keyEvent{kind: keyEsc}
	}

	switch b {
	case '[':
		return decodeCSI(r)
	case 'O':
		if r.Buffered() == 0 && !waitReadable(fd, 25*time.Millisecond) {
			return keyEvent{kind: keyEsc}
		}
		next, err := r.ReadByte()
		if err != nil {
			return keyEvent{kind: keyEsc}
		}
		return decodeFinal(next)
	default:
		_ = r.UnreadByte()
		return keyEvent{kind: keyEsc}
	}
}

func decodeCSI(r *bufio.Reader) keyEvent {
	var params []byte

	for {
		b, err := r.ReadByte()
		if err != nil {
			return keyEvent{kind: keyEsc}
		}
		if b >= 0x40 && b <= 0x7e {
			if b == '~' && string(params) == "200" {
				return readPaste(r)
			}
			switch string(params) {
			case "1", "7":
				return keyEvent{kind: keyHome}
			case "3":
				return keyEvent{kind: keyDelete}
			case "4", "8":
				return keyEvent{kind: keyEnd}
			case "5":
				return keyEvent{kind: keyHome}
			case "6":
				return keyEvent{kind: keyEnd}
			}
			return decodeFinal(b)
		}
		if len(params) < 32 {
			params = append(params, b)
		}
	}
}

func decodeFinal(b byte) keyEvent {
	switch b {
	case 'A':
		return keyEvent{kind: keyUp}
	case 'B':
		return keyEvent{kind: keyDown}
	case 'C':
		return keyEvent{kind: keyRight}
	case 'D':
		return keyEvent{kind: keyLeft}
	case 'H':
		return keyEvent{kind: keyHome}
	case 'F':
		return keyEvent{kind: keyEnd}
	}
	return keyEvent{}
}

func readPaste(r *bufio.Reader) keyEvent {
	const marker = "\x1b[201~"
	const limit = 1 << 20

	var data []byte
	for len(data) <= limit {
		b, err := r.ReadByte()
		if err != nil {
			break
		}
		data = append(data, b)
		if len(data) >= len(marker) && bytes.Equal(data[len(data)-len(marker):], []byte(marker)) {
			return keyEvent{kind: keyPaste, text: string(data[:len(data)-len(marker)])}
		}
	}
	return keyEvent{kind: keyPaste, text: string(data)}
}

func readRune(r *bufio.Reader, first byte) (rune, bool) {
	var buf [utf8.UTFMax]byte
	buf[0] = first

	size := 0
	switch {
	case first&0xe0 == 0xc0:
		size = 2
	case first&0xf0 == 0xe0:
		size = 3
	case first&0xf8 == 0xf0:
		size = 4
	default:
		return 0, false
	}

	for i := 1; i < size; i++ {
		b, err := r.ReadByte()
		if err != nil {
			return 0, false
		}
		buf[i] = b
	}

	value, n := utf8.DecodeRune(buf[:size])
	if value == utf8.RuneError && n == 1 {
		return 0, false
	}
	return value, true
}

func waitReadable(fd int, timeout time.Duration) bool {
	if fd < 0 || fd >= 1024 {
		time.Sleep(timeout)
		return false
	}

	var set syscall.FdSet
	set.Bits[fd/32] |= 1 << (uint(fd) % 32)
	tv := syscall.NsecToTimeval(timeout.Nanoseconds())

	if err := syscall.Select(fd+1, &set, nil, nil, &tv); err != nil {
		return false
	}
	return set.Bits[fd/32]&(1<<(uint(fd)%32)) != 0
}
