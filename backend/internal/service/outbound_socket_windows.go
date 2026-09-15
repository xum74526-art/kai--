//go:build windows

package service

import (
	"net"
	"os"
	"strings"
	"syscall"

	"golang.org/x/sys/windows"
)

const outboundDisableIPv4PMTUDEnv = "CANVAS_OUTBOUND_DISABLE_IPV4_PMTUD"

func configureOutboundDialer(dialer *net.Dialer) {
	if dialer == nil || !outboundIPv4FragmentationFallbackEnabled() {
		return
	}
	dialer.Control = func(network, _ string, raw syscall.RawConn) error {
		if network != "tcp4" {
			return nil
		}
		// Outline 等虚拟网卡可能报告 1500 MTU，但隧道封装后的真实路径更小。
		// 只对本进程的 IPv4 外连允许必要分片；选项不可用时沿用系统默认，避免兼容性补丁阻断请求。
		_ = raw.Control(func(fd uintptr) {
			_ = windows.SetsockoptInt(
				windows.Handle(fd),
				windows.IPPROTO_IP,
				windows.IP_MTU_DISCOVER,
				windows.IP_PMTUDISC_DONT,
			)
		})
		return nil
	}
}

func outboundIPv4FragmentationFallbackEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(outboundDisableIPv4PMTUDEnv))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}
