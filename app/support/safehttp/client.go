package safehttp

import (
	"context"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	frameworkbinding "github.com/goravel/framework/contracts/binding"
	frameworkfoundation "github.com/goravel/framework/foundation"

	"goravel/app/facades"
)

type Policy struct {
	InvalidURL     func() error
	UnresolvedHost func() error
	InvalidAddress func() error
	ValidateURL    func(*url.URL) error
	ValidateTarget func(*url.URL, []net.IP) error
}

func URL(raw string, policy Policy) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return nil, policy.InvalidURL()
	}
	if err := policy.ValidateURL(parsed); err != nil {
		return nil, err
	}
	ips, err := hostIPs(context.Background(), parsed.Hostname(), policy)
	if err != nil {
		return nil, err
	}
	if err := policy.ValidateTarget(parsed, ips); err != nil {
		return nil, err
	}
	return parsed, nil
}

func Client(timeout time.Duration, policy Policy) http.Client {
	transport := transport()
	transport.Proxy = nil
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		return dialContext(ctx, network, address, policy)
	}
	return http.Client{
		Timeout:   timeout,
		Transport: transport,
		CheckRedirect: func(req *http.Request, _ []*http.Request) error {
			_, err := URL(req.URL.String(), policy)
			return err
		},
	}
}

func IsPrivateIP(ip net.IP) bool {
	return ip == nil ||
		ip.IsLoopback() ||
		ip.IsPrivate() ||
		ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() ||
		ip.IsUnspecified()
}

func transport() *http.Transport {
	fallback := http.DefaultTransport.(*http.Transport).Clone()
	if frameworkfoundation.App == nil || !applicationHasBinding(frameworkbinding.Http) {
		return fallback
	}
	request := facades.Http().Client()
	if request == nil || request.HttpClient() == nil {
		return fallback
	}
	current, ok := request.HttpClient().Transport.(*http.Transport)
	if !ok || current == nil {
		return fallback
	}
	return current.Clone()
}

func applicationHasBinding(binding any) bool {
	for _, registered := range frameworkfoundation.App.Bindings() {
		if registered == binding {
			return true
		}
	}
	return false
}

func dialContext(ctx context.Context, network, address string, policy Policy) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, policy.InvalidAddress()
	}
	ips, err := hostIPs(ctx, host, policy)
	if err != nil {
		return nil, err
	}
	if err := policy.ValidateTarget(&url.URL{Host: address}, ips); err != nil {
		return nil, err
	}

	dialer := net.Dialer{}
	var lastErr error
	for _, ip := range ips {
		conn, err := dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
		if err == nil {
			return conn, nil
		}
		lastErr = err
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, policy.UnresolvedHost()
}

func hostIPs(ctx context.Context, host string, policy Policy) ([]net.IP, error) {
	host = strings.TrimSpace(host)
	if host == "" {
		return nil, policy.InvalidAddress()
	}
	if ip := net.ParseIP(host); ip != nil {
		return []net.IP{ip}, nil
	}
	ips, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
	if err != nil || len(ips) == 0 {
		return nil, policy.UnresolvedHost()
	}
	return ips, nil
}
