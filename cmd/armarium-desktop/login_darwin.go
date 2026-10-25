package main

/*
#cgo CFLAGS: -x objective-c -fobjc-arc -mmacosx-version-min=13.0
#cgo LDFLAGS: -framework Foundation -framework ServiceManagement
#import <Foundation/Foundation.h>
#import <ServiceManagement/ServiceManagement.h>
#include <stdlib.h>

// 0 not registered, 1 enabled, 2 waiting for approval in System Settings, 3 not found.
static int loginStatus(void) {
	return (int)[SMAppService mainAppService].status;
}

// Returns NULL on success, or an error message the caller frees.
static char *loginSet(int on) {
	NSError *err = nil;
	BOOL ok = on ? [[SMAppService mainAppService] registerAndReturnError:&err]
	             : [[SMAppService mainAppService] unregisterAndReturnError:&err];
	if (ok) return NULL;
	return strdup(err.localizedDescription.UTF8String ?: "unknown error");
}
*/
import "C"

import (
	"errors"
	"unsafe"

	"github.com/c0ze/armarium/internal/desktop"
)

// loginItem is the app's own login item (SMAppService.mainApp). macOS removes
// it when the app is deleted.
type loginItem struct{}

func newLoginItem() desktop.LoginItem { return loginItem{} }

func (loginItem) Enabled() bool {
	s := C.loginStatus()
	return s == 1 || s == 2
}

func (loginItem) SetEnabled(on bool) error {
	v := C.int(0)
	if on {
		v = 1
	}
	if msg := C.loginSet(v); msg != nil {
		defer C.free(unsafe.Pointer(msg))
		return errors.New(C.GoString(msg))
	}
	return nil
}
