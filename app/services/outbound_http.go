package services

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

type outboundHTTPPolicy struct {
	invalidURL     func() error
	unresolvedHost func() error
	invalidAddress func() error
	validateURL    func(*url.URL) error
	validateTarget func(*url.URL, []net.IP) error
}

func safeOutboundURL(raw string, policy outboundHTTPPolicy) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return nil, policy.invalidURL()
	}
	if err := policy.validateURL(parsed); err != nil {
		return nil, err
	}
	ips, err := safeOutboundHostIPs(context.Background(), parsed.Hostname(), policy)
	if err != nil {
		return nil, err
	}
	if err := policy.validateTarget(parsed, ips); err != nil {
		return nil, err
	}
	return parsed, nil
}

func safeOutboundHTTPClient(timeout time.Duration, policy outboundHTTPPolicy) http.Client {
	transport := outboundHTTPTransport()
	transport.Proxy = nil
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		return safeOutboundDialContext(ctx, network, address, policy)
	}
	return http.Client{
		Timeout:   timeout,
		Transport: transport,
		CheckRedirect: func(req *http.Request, _ []*http.Request) error {
			_, err := safeOutboundURL(req.URL.String(), policy)
			return err
		},
	}
}

func outboundHTTPTransport() *http.Transport {
	fallback := http.DefaultTransport.(*http.Transport).Clone()
	if frameworkfoundation.App == nil || !applicationHasBinding(frameworkbinding.Http) {
		return fallback
	}
	request := facades.Http().Client()
	if request == nil || request.HttpClient() == nil {
		return fallback
	}
	transport, ok := request.HttpClient().Transport.(*http.Transport)
	if !ok || transport == nil {
		return fallback
	}
	return transport.Clone()
}

func applicationHasBinding(binding any) bool {
	for _, registered := range frameworkfoundation.App.Bindings() {
		if registered == binding {
			return true
		}
	}
	return false
}

func safeOutboundDialContext(
	ctx context.Context,
	network string,
	address string,
	policy outboundHTTPPolicy,
) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, policy.invalidAddress()
	}
	ips, err := safeOutboundHostIPs(ctx, host, policy)
	if err != nil {
		return nil, err
	}
	if err := policy.validateTarget(&url.URL{Host: address}, ips); err != nil {
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
	return nil, policy.unresolvedHost()
}

func safeOutboundHostIPs(ctx context.Context, host string, policy outboundHTTPPolicy) ([]net.IP, error) {
	host = strings.TrimSpace(host)
	if host == "" {
		return nil, policy.invalidAddress()
	}
	if ip := net.ParseIP(host); ip != nil {
		return []net.IP{ip}, nil
	}
	ips, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
	if err != nil || len(ips) == 0 {
		return nil, policy.unresolvedHost()
	}
	return ips, nil
}

func isPrivateOutboundIP(ip net.IP) bool {
	return ip == nil ||
		ip.IsLoopback() ||
		ip.IsPrivate() ||
		ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() ||
		ip.IsUnspecified()
}
