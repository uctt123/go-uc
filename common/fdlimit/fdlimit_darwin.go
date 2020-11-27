















package fdlimit

import "syscall"


const hardlimit = 10240




func Raise(max uint64) (uint64, error) {
	
	var limit syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &limit); err != nil {
		return 0, err
	}
	
	limit.Cur = limit.Max
	if limit.Cur > max {
		limit.Cur = max
	}
	if err := syscall.Setrlimit(syscall.RLIMIT_NOFILE, &limit); err != nil {
		return 0, err
	}
	
	if err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &limit); err != nil {
		return 0, err
	}
	return limit.Cur, nil
}



func Current() (int, error) {
	var limit syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &limit); err != nil {
		return 0, err
	}
	return int(limit.Cur), nil
}



func Maximum() (int, error) {
	
	var limit syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &limit); err != nil {
		return 0, err
	}
	
	if limit.Max > hardlimit {
		limit.Max = hardlimit
	}
	return int(limit.Max), nil
}
