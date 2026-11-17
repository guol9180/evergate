//go:build darwin && !cgo

package desktop

func AcquireSingleInstance() (func(), bool, error) { return func() {}, true, nil }

func ShowError(string, string) {}

func RevealPath(string) error { return nil }

func StartTray(TrayActions) (func(), error) { return func() {}, nil }

func StopTray() {}
