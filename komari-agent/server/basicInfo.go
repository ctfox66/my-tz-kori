package server

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/komari-monitor/komari-agent/dnsresolver"
	monitoring "github.com/komari-monitor/komari-agent/monitoring/unit"
	"github.com/komari-monitor/komari-agent/protocol/transport"
	v2 "github.com/komari-monitor/komari-agent/protocol/v2"
	"github.com/komari-monitor/komari-agent/update"

	pkg_flags "github.com/komari-monitor/komari-agent/cmd/flags"
)

var flags = pkg_flags.GlobalConfig

func DoUploadBasicInfoWorks() {
	ticker := time.NewTicker(time.Duration(flags.InfoReportInterval) * time.Minute)
	for range ticker.C {
		err := uploadBasicInfo()
		if err != nil {
			log.Println("Error uploading basic info:", err)
		}
	}
}
func UpdateBasicInfo() {
	err := uploadBasicInfo()
	if err != nil {
		log.Println("Error uploading basic info:", err)
	} else {
		log.Println("Basic info uploaded successfully")
	}
}

// staticInfo caches the values that never change for the lifetime of the host.
// Building them on every report forced a CPU probe, a full disk partition scan
// and an outbound IP lookup once per interval, which showed up as periodic CPU
// spikes. They are now collected once and reused.
type basicStaticInfo struct {
	cpuName          string
	cpuCores         int
	cpuPhysicalCores int
	arch             string
	os               string
	kernelVersion    string
	memTotal         uint64
	swapTotal        uint64
	diskTotal        uint64
	gpuName          string
	virtualization   string
}

var (
	staticInfoOnce sync.Once
	staticInfo     basicStaticInfo
)

func getStaticInfo() basicStaticInfo {
	staticInfoOnce.Do(func() {
		cpuInfo := monitoring.CpuInfoStatic()
		staticInfo = basicStaticInfo{
			cpuName:          cpuInfo.CPUName,
			cpuCores:         cpuInfo.CPUCores,
			cpuPhysicalCores: cpuInfo.CPUPhysicalCores,
			arch:             cpuInfo.CPUArchitecture,
			os:               monitoring.OSName(),
			kernelVersion:    monitoring.KernelVersion(),
			memTotal:         monitoring.Ram().Total,
			swapTotal:        monitoring.Swap().Total,
			diskTotal:        monitoring.Disk().Total,
			gpuName:          monitoring.GpuName(),
			virtualization:   monitoring.Virtualized(),
		}
	})
	return staticInfo
}

// ipCache caches the resolved public IP addresses so that an unchanged host does
// not trigger an outbound lookup on every report.
type basicIPInfo struct {
	sync.Mutex
	ipv4    string
	ipv6    string
	fetched time.Time
	primed  bool
}

var ipInfoCache = &basicIPInfo{}

const ipCacheTTL = time.Hour

func getIPAddresses(force bool) (string, string) {
	ipInfoCache.Lock()
	defer ipInfoCache.Unlock()

	if !force && ipInfoCache.primed && time.Since(ipInfoCache.fetched) < ipCacheTTL {
		return ipInfoCache.ipv4, ipInfoCache.ipv6
	}

	ipv4, ipv6, _ := monitoring.GetIPAddress()
	ipInfoCache.ipv4, ipInfoCache.ipv6 = ipv4, ipv6
	ipInfoCache.fetched = time.Now()
	ipInfoCache.primed = true
	return ipv4, ipv6
}

func uploadBasicInfo() error {
	static := getStaticInfo()
	ipv4, ipv6 := getIPAddresses(false)

	data := map[string]interface{}{
		"cpu_name":           static.cpuName,
		"cpu_cores":          static.cpuCores,
		"cpu_physical_cores": static.cpuPhysicalCores,
		"arch":               static.arch,
		"os":                 static.os,
		"kernel_version":     static.kernelVersion,
		"ipv4":               ipv4,
		"ipv6":               ipv6,
		"mem_total":          static.memTotal,
		"swap_total":         static.swapTotal,
		"disk_total":         static.diskTotal,
		"gpu_name":           static.gpuName,
		"virtualization":     static.virtualization,
		"version":            update.CurrentVersion,
	}

	// 尝试上传完整数据
	err := tryUploadData(data)
	if err != nil {
		// 兼容 <= 1.0.2
		delete(data, "kernel_version")
		// 兼容 <= 1.2.0
		delete(data, "cpu_physical_cores")
		err = tryUploadData(data)
		if err != nil {
			return err
		}
	}
	return nil
}

func tryUploadData(data map[string]interface{}) error {
	protocolVersion := uploadProtocolVersion()
	if protocolVersion >= 2 {
		err := tryUploadDataWithProtocol(data, 2)
		if shouldFallbackToV1(2, err) {
			log.Printf("v2 basic info failed %d consecutive protocol attempts, falling back to v1", v2ProtocolFallbackThreshold)
			setConnectionProtocolVersion(1)
			return tryUploadDataWithProtocol(data, 1)
		}
		return err
	}
	return tryUploadDataWithProtocol(data, 1)
}

func tryUploadDataWithProtocol(data map[string]interface{}, protocolVersion int) error {
	endpoint := strings.TrimSuffix(flags.Endpoint, "/") + "/api/clients/uploadBasicInfo?token=" + flags.Token
	payload, err := json.Marshal(data)
	if err != nil {
		return err
	}
	if protocolVersion >= 2 {
		endpoint = strings.TrimSuffix(flags.Endpoint, "/") + "/api/clients/v2/rpc?token=" + flags.Token
		payload = v2.BuildBasicInfoPayload(data)
	}
	body := payload
	compressed := false
	if protocolVersion >= 2 && !flags.DisableCompression {
		if gz, err := transport.GzipBytes(payload); err == nil {
			body = gz
			compressed = true
		}
	}

	req, err := http.NewRequest("POST", endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if compressed {
		req.Header.Set("Content-Encoding", "gzip")
	}

	// 添加Cloudflare Access头部
	if flags.CFAccessClientID != "" && flags.CFAccessClientSecret != "" {
		req.Header.Set("CF-Access-Client-Id", flags.CFAccessClientID)
		req.Header.Set("CF-Access-Client-Secret", flags.CFAccessClientSecret)
	}

	client := dnsresolver.GetHTTPClient(30 * time.Second)

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	message := string(respBody)

	if resp.StatusCode != http.StatusOK {
		return &httpStatusError{StatusCode: resp.StatusCode, Status: resp.Status, Body: message}
	}
	if protocolVersion >= 2 {
		if len(bytes.TrimSpace(respBody)) > 0 {
			if _, err := parseV2Response(respBody); err != nil {
				return err
			}
		}
		resetV2ProtocolFailures(protocolVersion)
	}

	return nil
}
