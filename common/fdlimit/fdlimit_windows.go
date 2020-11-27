















package fdlimit

import "fmt"


const hardlimit = 16384



func Raise(max uint64) (uint64, error) {
	
	
	
	
	
	
	if max > hardlimit {
		return hardlimit, fmt.Errorf("file descriptor limit (%d) reached", hardlimit)
	}
	return max, nil
}



func Current() (int, error) {
	
	return hardlimit, nil
}



func Maximum() (int, error) {
	return Current()
}
