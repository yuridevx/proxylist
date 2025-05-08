package proxysource

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/coder/websocket"
	pgxtype "github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/yuridevx/poe2scrap/pkg/models"
	"go.uber.org/zap"
)

var ipCheckURLs = []string{
	"https://httpbin.org/ip",
	"https://api.ipify.org?format=json",
	"https://ifconfig.co/json",
}

type ProxyTest struct {
	db  *pgxpool.Pool
	log *zap.Logger
}

func NewProxyTest(log *zap.Logger, db *pgxpool.Pool) *ProxyTest {
	return &ProxyTest{log: log, db: db}
}

// tryGet does up to 2 attempts on temporary errors
func tryGet(client *http.Client, url string) (*http.Response, error) {
	var lastErr error
	for i := 0; i < 2; i++ {
		resp, err := client.Get(url)
		if err != nil {
			lastErr = err
			if ne, ok := err.(net.Error); ok && ne.Temporary() {
				time.Sleep(300 * time.Millisecond)
				continue
			}
			return nil, err
		}
		return resp, nil
	}
	return nil, lastErr
}

// anonymityCheck returns true if any IP-echo endpoint reports exactly proxyIP
func anonymityCheck(client *http.Client, proxyIP string) bool {
	for _, endpoint := range ipCheckURLs {
		resp, err := tryGet(client, endpoint)
		if err != nil {
			continue
		}
		raw, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		var m map[string]string
		if err := json.Unmarshal(raw, &m); err != nil {
			continue
		}

		origin := m["origin"]
		if origin == "" {
			origin = m["ip"]
		}
		for _, part := range strings.Split(origin, ",") {
			if strings.TrimSpace(part) == proxyIP {
				return true
			}
		}
	}
	return false
}

func (pt *ProxyTest) Test(ctx context.Context, ip string, port int32, protocol string) (err error) {
	q := models.New(pt.db)
	var testError string
	defer func() {
		notify := models.NotifyProxyTestedParams{
			Ip:       ip,
			Port:     port,
			Protocol: protocol,
			ProcessedAt: pgxtype.Timestamp{
				Time:  time.Now(),
				Valid: true,
			},
			TestedAt: pgxtype.Timestamp{
				Time:  time.Now(),
				Valid: true,
			},
			AllowRetryAt: pgxtype.Timestamp{
				Time:  time.Now().Add(24 * time.Hour),
				Valid: true,
			},
			TestError: pgxtype.Text{
				String: testError,
				Valid:  testError != "",
			},
		}
		if ne := q.NotifyProxyTested(context.Background(), notify); ne != nil {
			pt.log.Error("NotifyProxyTested failed", zap.Error(ne))
		}
	}()

	addr := fmt.Sprintf("%s:%d", ip, port)

	// 1) quick TCP check
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	start := time.Now()
	if _, err = dialer.DialContext(ctx, "tcp", addr); err != nil {
		testError = err.Error()
		return err
	}
	delay := time.Since(start)

	// 2) build HTTP transport
	var transport *http.Transport
	tlsCfg := &tls.Config{InsecureSkipVerify: true}
	switch protocol {
	case "http", "https":
		pu := &url.URL{Scheme: protocol, Host: addr}
		transport = &http.Transport{
			Proxy:           http.ProxyURL(pu),
			TLSClientConfig: tlsCfg,
		}

	//case "socks5", "socks4", "socks4a":
	//	pu := &url.URL{Scheme: protocol, Host: addr}
	//	sd, derr := proxy.FromURL(pu, proxy.Direct)
	//	if derr != nil {
	//		pt.log.Error("SOCKS dialer failed", zap.Error(derr))
	//		testError = derr.Error()
	//		return derr
	//	}
	//	transport = &http.Transport{
	//		DialContext:     func(_ context.Context, _, target string) (net.Conn, error) { return sd.Dial("tcp", target) },
	//		TLSClientConfig: tlsCfg,
	//	}

	default:
		err = fmt.Errorf("unsupported protocol %q", protocol)
		testError = err.Error()
		return err
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   10 * time.Second,
	}

	// 3) WebSocket echo test (retry on temporary WS failures)
	wsSupported := false
	for i := 0; i < 2; i++ {
		conn, _, wserr := websocket.Dial(ctx, "wss://echo.websocket.events",
			&websocket.DialOptions{HTTPClient: client},
		)
		if wserr == nil {
			wsSupported = true
			conn.Close(websocket.StatusNormalClosure, "")
			break
		}
		if ne, ok := wserr.(net.Error); ok && ne.Temporary() {
			time.Sleep(300 * time.Millisecond)
			continue
		}
		//pt.log.Warn("WebSocket dial failed", zap.Error(wserr))
		break
	}

	// 4) anonymity check
	anonymity := anonymityCheck(client, ip)

	// 5) basic GET
	itemFetch := false
	if resp, ferr := tryGet(client, "https://httpbin.org/get"); ferr == nil {
		itemFetch = resp.StatusCode == http.StatusOK
		resp.Body.Close()
	}

	// 6) persist detailed results
	insert := models.InsertProxyInfoTestResultsParams{
		Ip:        ip,
		Port:      port,
		Protocol:  protocol,
		Delay:     pgxtype.Interval{Microseconds: delay.Microseconds(), Valid: true},
		TestedAt:  pgxtype.Timestamp{Time: time.Now(), Valid: true},
		Websocket: pgxtype.Bool{Bool: wsSupported, Valid: true},
		Anonymity: pgxtype.Bool{Bool: anonymity, Valid: true},
		ItemFetch: pgxtype.Bool{Bool: itemFetch, Valid: true},
	}
	if ierr := q.InsertProxyInfoTestResults(context.Background(), insert); ierr != nil {
		//pt.log.Error("InsertProxyInfoTestResults failed", zap.Error(ierr))
		testError = ierr.Error()
	}

	return nil
}
