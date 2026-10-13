//go:build !windows

package outbound

import "net"

func configureOutboundDialer(_ *net.Dialer) {}
