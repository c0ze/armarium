//go:build !darwin && !windows

package desktop

import (
	"context"
	"errors"
)

func chooseFolder(context.Context) (string, error) {
	return "", errors.New("type the folder's path instead")
}
