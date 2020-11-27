















package params

import (
	"fmt"
)

const (
	VersionMajor = 1        
	VersionMinor = 9        
	VersionPatch = 23       
	VersionMeta  = "stable" 
)


var Version = func() string {
	return fmt.Sprintf("%d.%d.%d", VersionMajor, VersionMinor, VersionPatch)
}()


var VersionWithMeta = func() string {
	v := Version
	if VersionMeta != "" {
		v += "-" + VersionMeta
	}
	return v
}()




func ArchiveVersion(gitCommit string) string {
	vsn := Version
	if VersionMeta != "stable" {
		vsn += "-" + VersionMeta
	}
	if len(gitCommit) >= 8 {
		vsn += "-" + gitCommit[:8]
	}
	return vsn
}

func VersionWithCommit(gitCommit, gitDate string) string {
	vsn := VersionWithMeta
	if len(gitCommit) >= 8 {
		vsn += "-" + gitCommit[:8]
	}
	if (VersionMeta != "stable") && (gitDate != "") {
		vsn += "-" + gitDate
	}
	return vsn
}
