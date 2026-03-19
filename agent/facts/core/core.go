// Package core provides core system facts.
package core

import (
	"github.com/guilledipa/praetor/agent/facts"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/host"
	"github.com/shirou/gopsutil/v3/net"
	"os"
	"runtime"
)

// CoreFacter provides basic system facts.
type CoreFacter struct{}

// Name returns the name of the facter.
func (f *CoreFacter) Name() string {
	return "core"
}

// GetFacts returns core system facts.
func (f *CoreFacter) GetFacts() (map[string]any, error) {
	fcts := make(map[string]any)

	// Basic runtime
	fcts["os"] = runtime.GOOS
	fcts["arch"] = runtime.GOARCH
	fcts["num_cpu"] = runtime.NumCPU()
	fcts["go_version"] = runtime.Version()
	fcts["pid"] = os.Getpid()

	hostname, err := os.Hostname()
	if err == nil {
		fcts["hostname"] = hostname
	} else {
		fcts["hostname"] = "unknown"
	}

	// Host Info
	if info, err := host.Info(); err == nil {
		fcts["host_id"] = info.HostID
		fcts["uptime"] = info.Uptime
		fcts["boot_time"] = info.BootTime
		fcts["platform"] = info.Platform
		fcts["platform_family"] = info.PlatformFamily
		fcts["platform_version"] = info.PlatformVersion
		fcts["kernel_version"] = info.KernelVersion
		fcts["virtualization_system"] = info.VirtualizationSystem
		fcts["virtualization_role"] = info.VirtualizationRole
	}

	// CPU Info
	if cpuInfo, err := cpu.Info(); err == nil && len(cpuInfo) > 0 {
		fcts["cpu_model_name"] = cpuInfo[0].ModelName
		fcts["cpu_cores"] = cpuInfo[0].Cores
	}
	if counts, err := cpu.Counts(true); err == nil {
		fcts["cpu_logical_count"] = counts
	}
	if counts, err := cpu.Counts(false); err == nil {
		fcts["cpu_physical_count"] = counts
	}

	// Network Interfaces
	if interfaces, err := net.Interfaces(); err == nil {
		netInfo := make(map[string]any)
		for _, iface := range interfaces {
			if len(iface.Addrs) > 0 {
				var ips []string
				for _, addr := range iface.Addrs {
					ips = append(ips, addr.Addr)
				}
				netInfo[iface.Name] = map[string]any{
					"mac":   iface.HardwareAddr,
					"ips":   ips,
					"mtu":   iface.MTU,
					"flags": iface.Flags,
				}
			}
		}
		fcts["network_interfaces"] = netInfo
	}

	return fcts, nil
}

func init() {
	facts.RegisterFacter(&CoreFacter{})
}
