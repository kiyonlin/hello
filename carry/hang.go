package carry

import "sync"

var lastHangTime = 0

type hangStatus struct {
	lock sync.Mutex
}
