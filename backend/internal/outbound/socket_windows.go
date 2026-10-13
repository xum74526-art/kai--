//go:build windows

package outbound

import (
	"net"
	"os"
	"strings"
	"syscall"

	"golang.org/x/sys/windows"
)

func configureOutboundDialer(dialer *net.Dialer) {
	value := strings.ToLower(strings.TrimSpace(os.Getenv("CANVAS_OUTBOUND_DISABLE_IPV4_PMTUD")))
	if dialer == nil || (value != "1" && value != "true" && value != "yes" && value != "on") {
		return
	}
	dialer.Control = func(network, _ string, raw syscall.RawConn) error {
		if network != "tcp4" {
			return nil
		}
		// 保留虚拟网卡大请求的本进程分片开关，未开启时不改变系统网络行为。
		_ = raw.Control(func(fd uintptr) {
			_ = windows.SetsockoptInt(windows.Handle(fd), windows.IPPROTO_IP, windows.IP_MTU_DISCOVER, windows.IP_PMTUDISC_DONT)
		})
		return nil
	}
}
