// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package connections

import (
	"net"
	"net/url"
	"strconv"
	"strings"
)

//Private IPv4 ranges
var (
	_, private24BitBlock, _ = net.ParseCIDR("10.0.0.0/8")
    _, private20BitBlock, _ = net.ParseCIDR("172.16.0.0/12")
    _, private16BitBlock, _ = net.ParseCIDR("192.168.0.0/16")
)


func fixupPort(uri *url.URL, defaultPort int) *url.URL {
	copyURI := *uri

	host, port, err := net.SplitHostPort(uri.Host)
	if err != nil && strings.Contains(err.Error(), "missing port") {
		// addr is on the form "1.2.3.4"
		copyURI.Host = net.JoinHostPort(uri.Host, strconv.Itoa(defaultPort))
	} else if err == nil && port == "" {
		// addr is on the form "1.2.3.4:"
		copyURI.Host = net.JoinHostPort(host, strconv.Itoa(defaultPort))
	}

	return &copyURI
}

/*
 * addr = <ip>:<port>
 */
func isAddressLocal(addr string) bool {
	/*if f.model.conn[selected.ID].Type() == "relay-client" || f.model.conn[selected.ID].Type() == "relay-server" {
		l.Infoln("Not requesting remote data over relay")
		continue
	}*/
	isLocal := true
	if strings.HasPrefix(addr, "[") {//implies ipv6
		isLocal = strings.HasPrefix(addr, "[fe80")
	} else {
		ipStr := strings.SplitN(addr,":",2)[0]//w.x.y.z
		ip := net.ParseIP(ipStr)
		isLocal = private24BitBlock.Contains(ip) || private20BitBlock.Contains(ip) || private16BitBlock.Contains(ip)
	}
	return isLocal
}