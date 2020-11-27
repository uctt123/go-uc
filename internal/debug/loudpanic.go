

















package debug

import "runtime/debug"


func LoudPanic(x interface{}) {
	debug.SetTraceback("all")
	panic(x)
}
