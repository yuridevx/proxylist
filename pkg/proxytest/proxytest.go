package proxytest

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/yuridevx/proxylist/domain"
	"h12.io/socks"
)

// Protocol is an "enum" of the transports we test.
type Protocol int

const (
	ProtoHTTP Protocol = iota
	ProtoHTTPS
	ProtoSOCKS4
	ProtoSOCKS4A
	ProtoSOCKS5
)

func (p Protocol) String() string {
	switch p {
	case ProtoHTTP:
		return "http"
	case ProtoHTTPS:
		return "https"
	case ProtoSOCKS4:
		return "socks4"
	case ProtoSOCKS4A:
		return "socks4a"
	case ProtoSOCKS5:
		return "socks5"
	default:
		return "unknown"
	}
}

// ProtocolResult holds the outcome of one protocol test.
type ProtocolResult struct {
	Success      bool
	Duration     time.Duration
	Error        error
	WebSocket    *ProtocolResult // nil if WS wasn't attempted
	ExposesIP    bool            // true if any IP‐leak header was present
	FetchSuccess bool            // true if custom URL GET succeeded
}

// BestResult bundles which protocol was chosen and its result.
type BestResult struct {
	Proto Protocol
	ProtocolResult
}

// ProxyChecker knows how to test proxies.
type ProxyChecker struct {
	Timeout       time.Duration
	HTTPBinGetURL string
	HTTPBinIPURL  string
	WebSocketURL  string
	FetchURL      string
}

// NewProxyChecker returns a checker with sensible defaults.
// Pass your custom URL as the argument.
func NewProxyChecker(fetchURL string, timeoutS int) *ProxyChecker {
	return &ProxyChecker{
		Timeout:       time.Duration(timeoutS) * time.Second,
		HTTPBinGetURL: "http://httpbin.org/get",
		HTTPBinIPURL:  "https://httpbin.org/ip",
		WebSocketURL:  "ws://echo.websocket.org",
		FetchURL:      fetchURL,
	}
}

// Check tests HTTP, HTTPS, SOCKS4, SOCKS4A, and SOCKS5 in parallel,
// then returns only the BestResult according to:
//  1. If any WebSocket tests succeeded, pick the one with the fastest WS.Duration
//  2. Otherwise, pick highest-priority success: socks5 > socks4a > socks4 > https > http
func (pc *ProxyChecker) Check(ctx context.Context, p domain.ProvidedProxy) (BestResult, error) {
	addr := fmt.Sprintf("%s:%d", p.IP, p.Port)
	type item struct {
		code Protocol
		pr   ProtocolResult
	}
	resultsCh := make(chan item, 5)
	var wg sync.WaitGroup

	// HTTP
	wg.Add(1)
	go func() {
		defer wg.Done()
		ok, dur, exposes, err := pc.checkHTTP(ctx, addr)
		pr := ProtocolResult{Success: ok, Duration: dur, Error: err, ExposesIP: exposes}
		if ok {
			// WebSocket over HTTP proxy
			ws := pc.runWS(ctx, "http", addr)
			pr.WebSocket = &ws
			// custom URL GET
			pr.FetchSuccess = pc.checkFetch(ctx, "http", addr)
		}
		resultsCh <- item{ProtoHTTP, pr}
	}()

	// HTTPS
	wg.Add(1)
	go func() {
		defer wg.Done()
		ok, dur, err := pc.checkHTTPS(ctx, addr)
		pr := ProtocolResult{Success: ok, Duration: dur, Error: err}
		if ok {
			ws := pc.runWS(ctx, "https", addr)
			pr.WebSocket = &ws
			pr.FetchSuccess = pc.checkFetch(ctx, "https", addr)
		}
		resultsCh <- item{ProtoHTTPS, pr}
	}()

	// SOCKS4, SOCKS4A, SOCKS5
	for _, v := range []struct {
		name string
		code Protocol
	}{{"socks4", ProtoSOCKS4}, {"socks4a", ProtoSOCKS4A}, {"socks5", ProtoSOCKS5}} {
		wg.Add(1)
		go func(name string, code Protocol) {
			defer wg.Done()
			ok, dur, err := pc.checkSOCKS(ctx, addr, name)
			pr := ProtocolResult{Success: ok, Duration: dur, Error: err}
			if ok {
				ws := pc.runWS(ctx, name, addr)
				pr.WebSocket = &ws
				pr.FetchSuccess = pc.checkFetch(ctx, name, addr)
			}
			resultsCh <- item{code, pr}
		}(v.name, v.code)
	}

	wg.Wait()
	close(resultsCh)

	// gather items
	var items []item
	for r := range resultsCh {
		items = append(items, r)
	}

	// 1) WebSocket priority
	var wsSuccess []item
	for _, it := range items {
		if it.pr.WebSocket != nil && it.pr.WebSocket.Success {
			wsSuccess = append(wsSuccess, it)
		}
	}
	if len(wsSuccess) > 0 {
		best := wsSuccess[0]
		for _, it := range wsSuccess[1:] {
			if it.pr.WebSocket.Duration < best.pr.WebSocket.Duration {
				best = it
			}
		}
		return BestResult{Proto: best.code, ProtocolResult: best.pr}, nil
	}

	// 2) fixed priority
	priority := []Protocol{ProtoSOCKS5, ProtoSOCKS4A, ProtoSOCKS4, ProtoHTTPS, ProtoHTTP}
	for _, proto := range priority {
		for _, it := range items {
			if it.code == proto && it.pr.Success {
				return BestResult{Proto: proto, ProtocolResult: it.pr}, nil
			}
		}
	}

	// 3) fallback
	if len(items) > 0 {
		it := items[0]
		return BestResult{Proto: it.code, ProtocolResult: it.pr}, nil
	}

	return BestResult{}, nil
}

// checkFetch performs a simple GET of the FetchURL through the given proxy.
// It returns true if the request succeeds (status code < 400).
func (pc *ProxyChecker) checkFetch(ctx context.Context, proto, proxyAddr string) bool {
	tr := &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}
	// set proxy or dialer
	if proto == "http" || proto == "https" {
		tr.Proxy = http.ProxyURL(&url.URL{Scheme: "http", Host: proxyAddr})
	} else {
		dial := socks.Dial(fmt.Sprintf("%s://%s?timeout=%s", proto, proxyAddr, pc.Timeout))
		tr.DialContext = func(_ context.Context, network, addr string) (net.Conn, error) {
			return dial(network, addr)
		}
	}
	client := &http.Client{Transport: tr, Timeout: pc.Timeout}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, pc.FetchURL, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/134.0.0.0 Safari/537.36")
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	if strings.Contains(resp.Header.Get("Set-Cookie"), "POESESSID") {
		return true
	}
	return false
}

// runWS runs a WebSocket handshake over the given proxy. It measures
// the time to Dial, then closes the connection with a normal close code.
func (pc *ProxyChecker) runWS(ctx context.Context, proto, proxyAddr string) ProtocolResult {
	start := time.Now()

	// 1) Build an http.Transport that uses your proxy:
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	if proto == "http" || proto == "https" {
		// HTTP(S) CONNECT proxy
		tr.Proxy = http.ProxyURL(&url.URL{Scheme: "http", Host: proxyAddr})
	} else {
		// SOCKS4/4a/5 proxy
		dial := socks.Dial(fmt.Sprintf("%s://%s", proto, proxyAddr))
		tr.DialContext = func(_ context.Context, network, addr string) (net.Conn, error) {
			return dial(network, addr)
		}
	}

	client := &http.Client{
		Transport: tr,
		Timeout:   pc.Timeout,
	}

	// 2) Use coder/websocket to perform the WebSocket handshake over that client:
	conn, _, err := websocket.Dial(ctx, pc.WebSocketURL, &websocket.DialOptions{
		HTTPClient: client,
	}) // :contentReference[oaicite:0]{index=0}
	dur := time.Since(start)

	// 3) Clean up if we connected:
	if conn != nil {
		_ = conn.Close(websocket.StatusNormalClosure, "") // normal closure
	}

	return ProtocolResult{
		Success:  err == nil,
		Duration: dur,
		Error:    err,
	}
}

// checkHTTP tests plain HTTP for success, latency, and IP‐leak headers.
func (pc *ProxyChecker) checkHTTP(ctx context.Context, proxyAddr string) (bool, time.Duration, bool, error) {
	start := time.Now()
	tr := &http.Transport{Proxy: http.ProxyURL(&url.URL{Scheme: "http", Host: proxyAddr}), TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}
	client := &http.Client{Transport: tr, Timeout: pc.Timeout}

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, pc.HTTPBinGetURL, nil)
	resp, err := client.Do(req)
	dur := time.Since(start)
	if err != nil {
		return false, dur, false, err
	}
	defer resp.Body.Close()

	var body struct{ Headers map[string]string }
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return false, dur, false, err
	}

	for _, h := range []string{"X-Forwarded-For", "X-Real-IP", "Forwarded", "Client-IP", "Forwarded-For", "True-Client-IP", "CF-Connecting-IP", "Fastly-Client-Ip", "X-Cluster-Client-IP", "X-Forwarded", "Forwarded-For-Ip"} {
		if val, ok := body.Headers[h]; ok && val != "" {
			return true, dur, true, nil
		}
	}
	return true, dur, false, nil
}

// checkHTTPS tests HTTPS connectivity via the proxy.
func (pc *ProxyChecker) checkHTTPS(ctx context.Context, proxyAddr string) (bool, time.Duration, error) {
	start := time.Now()
	tr := &http.Transport{Proxy: http.ProxyURL(&url.URL{Scheme: "http", Host: proxyAddr}), TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}
	client := &http.Client{Transport: tr, Timeout: pc.Timeout}
	_, err := client.Get(pc.HTTPBinIPURL)
	return err == nil, time.Since(start), err
}

// checkSOCKS tests a single SOCKS proxy by issuing an HTTP GET.
func (pc *ProxyChecker) checkSOCKS(ctx context.Context, proxyAddr, proto string) (bool, time.Duration, error) {
	start := time.Now()
	dial := socks.Dial(fmt.Sprintf("%s://%s?timeout=%s", proto, proxyAddr, pc.Timeout))
	tr := &http.Transport{DialContext: func(_ context.Context, network, addr string) (net.Conn, error) { return dial(network, addr) }, TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}
	client := &http.Client{Transport: tr, Timeout: pc.Timeout}
	_, err := client.Get(pc.HTTPBinGetURL)
	return err == nil, time.Since(start), err
}
