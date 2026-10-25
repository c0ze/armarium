//go:build !darwin && !windows

package main

import "github.com/c0ze/armarium/internal/desktop"

func newLoginItem() desktop.LoginItem { return nil }
