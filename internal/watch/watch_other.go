//go:build !darwin

package watch

func New() Watcher { return unsupported{} }

type unsupported struct{}

func (unsupported) Foreground() (ForegroundApp, error) { return ForegroundApp{}, ErrUnsupported }
func (unsupported) Supported() bool                    { return false }
func (unsupported) Name() string                       { return "unsupported" }
