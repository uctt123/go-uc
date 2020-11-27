















package netutil


func IsTemporaryError(err error) bool {
	tempErr, ok := err.(interface {
		Temporary() bool
	})
	return ok && tempErr.Temporary() || isPacketTooBig(err)
}


func IsTimeout(err error) bool {
	timeoutErr, ok := err.(interface {
		Timeout() bool
	})
	return ok && timeoutErr.Timeout()
}
