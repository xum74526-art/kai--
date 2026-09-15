//go:build !windows

package service

import "net"

func configureOutboundDialer(_ *net.Dialer) {}
