package monitoring

import (
	"bufio"
	"os"
	"runtime"
	"strings"
	"sync"

	pkg_flags "github.com/komari-monitor/komari-agent/cmd/flags"
	"github.com/shirou/gopsutil/v4/cpu"
)

var flags = pkg_flags.GlobalConfig

type CpuInfo struct {
	CPUName          string  `json:"cpu_name"`
	CPUArchitecture  string  `json:"cpu_architecture"`
	CPUCores         int     `json:"cpu_cores"`
	CPUPhysicalCores int     `json:"cpu_physical_cores"`
	CPUUsage         float64 `json:"cpu_usage"`
}

var (
	cpuInfoOnce  sync.Once
	cpuInfoCache CpuInfo
)

// CpuInfoStatic returns the immutable CPU description (model name, core counts).
// It is computed once and cached, because these values never change at runtime
// and querying them may shell out or parse large files on some platforms.
func CpuInfoStatic() CpuInfo {
	cpuInfoOnce.Do(func() {
		info := CpuInfo{
			CPUName:          "Unknown",
			CPUArchitecture:  runtime.GOARCH,
			CPUCores:         1,
			CPUPhysicalCores: 0, // 为兼容旧版 agent，0 表示未上报或未知，避免与实际核心数混淆
			CPUUsage:         0.0,
		}

		// 优先使用 gopsutil 获取 CPU 信息，避免触发 lscpu 在部分内核上的 lockdown 日志刷屏。
		hwInfo, err := cpu.Info()
		if err == nil && len(hwInfo) > 0 {
			info.CPUName = strings.TrimSpace(hwInfo[0].ModelName)
			if info.CPUName == "" {
				if hwInfo[0].VendorID != "" || hwInfo[0].Family != "" {
					info.CPUName = strings.TrimSpace(hwInfo[0].VendorID + " " + hwInfo[0].Family)
				}
			}
		}

		if info.CPUName == "Unknown" {
			name, nameErr := readCPUNameFromProc()
			if nameErr == nil && name != "" {
				info.CPUName = strings.TrimSpace(name)
			}
		}

		cores, coresErr := cpu.Counts(true)
		if coresErr == nil && cores > 0 {
			info.CPUCores = cores
		}

		physicalCores, physicalErr := cpu.Counts(false)
		if physicalErr == nil && physicalCores > 0 {
			info.CPUPhysicalCores = physicalCores
		}

		cpuInfoCache = info
	})
	return cpuInfoCache
}

// Cpu returns CPU information together with a non-blocking usage sample.
// The usage is derived from consecutive /proc/stat deltas, so this call no
// longer blocks for one second.
func Cpu() CpuInfo {
	cpuinfo := CpuInfoStatic()
	cpuinfo.CPUUsage = CPUUsage()
	return cpuinfo
}

// readCPUNameFromProc 从 /proc/cpuinfo 读取 CPU 名称
func readCPUNameFromProc() (string, error) {
	file, err := os.Open("/proc/cpuinfo")
	if err != nil {
		return "", err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "Model\t") || strings.HasPrefix(line, "Hardware\t") || strings.HasPrefix(line, "Processor\t") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				return strings.TrimSpace(parts[1]), nil
			}
		}
	}

	return "", scanner.Err()
}
