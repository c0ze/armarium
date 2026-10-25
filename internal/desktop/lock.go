package desktop

import "errors"

// ErrRunning means another Armarium already holds the data folder.
var ErrRunning = errors.New("armarium is already running")
