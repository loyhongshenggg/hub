package reader

import (
	"errors"
	"net"
	"net/http"
	"net/netip"
	"syscall"
	"time"
)

var errPrivate = errors.New("refusing to fetch a private or local address")

var cgnat = netip.MustParsePrefix("100.64.0.0/10")

// SafeClient fetches only public addresses: links come from third-party
// feeds, so a reader must not become a way into localhost, the hub or the
// LAN. The check runs on every connection, so redirects are covered too.
func SafeClient() *http.Client {
	d := &net.Dialer{
		Timeout: 10 * time.Second,
		Control: func(_, address string, _ syscall.RawConn) error {
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return err
			}
			ip, err := netip.ParseAddr(host)
			if err != nil {
				return err
			}
			ip = ip.Unmap()
			if !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || cgnat.Contains(ip) {
				return errPrivate
			}
			return nil
		},
	}
	tr := &http.Transport{
		Proxy:                 nil, // a proxy would bypass the address check
		DialContext:           d.DialContext,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 15 * time.Second,
		MaxIdleConns:          10,
		IdleConnTimeout:       60 * time.Second,
	}
	return &http.Client{
		Transport: tr,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return errors.New("too many redirects")
			}
			if req.URL.Scheme != "https" && req.URL.Scheme != "http" {
				return errors.New("redirect to a non-web link")
			}
			return nil
		},
	}
}
