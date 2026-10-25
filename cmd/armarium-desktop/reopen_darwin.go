package main

/*
#cgo LDFLAGS: -framework Cocoa
void listenForReopen(void);
*/
import "C"

var onReopen func()

//export goReopen
func goReopen() {
	if onReopen != nil {
		go onReopen()
	}
}

// listenForReopen calls f when the app is opened again while it runs.
func listenForReopen(f func()) {
	onReopen = f
	C.listenForReopen()
}
