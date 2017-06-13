// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package connections

import (
	"crypto/tls"
	"net/url"
	"time"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/dialer"
	"github.com/syncthing/syncthing/lib/protocol"
)

func init() {
	factory := &tcpLocalDialerFactory{}
	for _, scheme := range []string{"tcp-local", "tcp4-local", "tcp6-local"} {
		dialers[scheme] = factory
	}
}

type tcpLocalDialer struct {
	cfg    *config.Wrapper
	tlsCfg *tls.Config
}

func (d *tcpLocalDialer) Dial(id protocol.DeviceID, uri *url.URL) (internalConn, error) {
	uri = fixupPort(uri, config.DefaultTCPPort)

	conn, err := dialer.DialTimeout(uri.Scheme, uri.Host, 10*time.Second)
	if err != nil {
		l.Debugln(err)
		return internalConn{}, err
	}

	err = dialer.SetTCPOptions(conn)
	if err != nil {
		l.Infoln(err)
	}

	err = dialer.SetTrafficClass(conn, d.cfg.Options().TrafficClass)
	if err != nil {
		l.Debugf("failed to set traffic class: %s", err)
	}

	tc := tls.Client(conn, d.tlsCfg)
	err = tlsTimedHandshake(tc)
	if err != nil {
		tc.Close()
		return internalConn{}, err
	}

	return internalConn{tc, connTypeTCPLocalClient, tcpLocalPriority}, nil
}

func (d *tcpLocalDialer) RedialFrequency() time.Duration {
	return time.Duration(d.cfg.Options().ReconnectIntervalS) * time.Second
}

type tcpLocalDialerFactory struct{}

func (tcpLocalDialerFactory) New(cfg *config.Wrapper, tlsCfg *tls.Config) genericDialer {
	return &tcpLocalDialer{
		cfg:    cfg,
		tlsCfg: tlsCfg,
	}
}

func (tcpLocalDialerFactory) Priority() int {
	return tcpLocalPriority
}

func (tcpLocalDialerFactory) Enabled(cfg config.Configuration) bool {
	return true
}

func (tcpLocalDialerFactory) String() string {
	return "TCP Local Dialer"
}
