//go:build !windows

package app

import "errors"

func openProjectLocation(path string, folder bool, action string) error {
	return errors.New("opening Explorer or Terminal is available on Windows")
}
