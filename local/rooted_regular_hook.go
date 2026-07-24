package local

import "sync"

var rootedRegularOpenHook struct {
	sync.Mutex
	callback func(string)
}

func runRootedRegularOpenHook(name string) {
	rootedRegularOpenHook.Lock()
	callback := rootedRegularOpenHook.callback
	rootedRegularOpenHook.Unlock()
	if callback != nil {
		callback(name)
	}
}

func setRootedRegularOpenHookForTest(callback func(string)) func() {
	rootedRegularOpenHook.Lock()
	previous := rootedRegularOpenHook.callback
	rootedRegularOpenHook.callback = callback
	rootedRegularOpenHook.Unlock()
	return func() {
		rootedRegularOpenHook.Lock()
		rootedRegularOpenHook.callback = previous
		rootedRegularOpenHook.Unlock()
	}
}
