// Package version 在编译期由 ldflags 注入版本信息。
package version

var (
	Build = "dev"
	Time  = "unknown"
)

// Info 返回版本字符串。
func Info() string {
	return "build=" + Build + " time=" + Time
}
