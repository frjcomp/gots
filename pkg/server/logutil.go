package server

import (
	"fmt"
	"strings"
)

func isBenignCloseError(err error) bool {
	if err == nil {
		return false
	}
	s := fmt.Sprint(err)
	return strings.Contains(s, "use of closed network connection") ||
		strings.Contains(s, "read/write on closed pipe")
}
