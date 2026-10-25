package desktop

import (
	"errors"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"
)

// mutexName is what the installer's AppMutex waits on before replacing files.
const mutexName = "ArmariumDesktopRunning"

// Lock takes the data folder for this process; release gives it back. It
// also holds the named mutex the installer and uninstaller look for.
func Lock(dir string) (release func(), err error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(dir, "armarium.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	h := windows.Handle(f.Fd())
	ol := new(windows.Overlapped)
	if err := windows.LockFileEx(h, windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, ol); err != nil {
		f.Close()
		if errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
			return nil, ErrRunning
		}
		return nil, err
	}
	name, _ := windows.UTF16PtrFromString(mutexName)
	mutex, _ := windows.CreateMutex(nil, false, name)
	return func() {
		if mutex != 0 {
			windows.CloseHandle(mutex)
		}
		windows.UnlockFileEx(h, 0, 1, 0, ol)
		f.Close()
	}, nil
}
