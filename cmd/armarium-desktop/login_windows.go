package main

import (
	"os"

	"github.com/c0ze/armarium/internal/desktop"
	"golang.org/x/sys/windows/registry"
)

const runKey = `Software\Microsoft\Windows\CurrentVersion\Run`

// loginItem is the "Armarium" value under the user's Run key; the uninstaller
// removes it.
type loginItem struct{}

func newLoginItem() desktop.LoginItem { return loginItem{} }

func (loginItem) Enabled() bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	_, _, err = k.GetStringValue("Armarium")
	return err == nil
}

func (loginItem) SetEnabled(on bool) error {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, runKey, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	if !on {
		if err := k.DeleteValue("Armarium"); err != nil && err != registry.ErrNotExist {
			return err
		}
		return nil
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	return k.SetStringValue("Armarium", `"`+exe+`"`)
}
