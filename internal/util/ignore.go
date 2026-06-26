package util

func Ignore(_ ...any) {}

func IgnoreErr(fn func() error) {
	_ = fn()
}
