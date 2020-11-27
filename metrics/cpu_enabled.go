

















package metrics

import (
	"github.com/ethereum/go-ethereum/log"
	"github.com/shirou/gopsutil/cpu"
)


func ReadCPUStats(stats *CPUStats) {
	
	timeStats, err := cpu.Times(false)
	if err != nil {
		log.Error("Could not read cpu stats", "err", err)
		return
	}
	
	timeStat := timeStats[0]
	stats.GlobalTime = int64((timeStat.User + timeStat.Nice + timeStat.System) * cpu.ClocksPerSec)
	stats.GlobalWait = int64((timeStat.Iowait) * cpu.ClocksPerSec)
	stats.LocalTime = getProcessCPUTime()
}
