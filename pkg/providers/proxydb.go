package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/yuridevx/proxylist/domain"
	"github.com/yuridevx/proxylist/pkg/utils"
	"net/http"
	"strconv"
	"time"

	"github.com/cenkalti/backoff/v5"
	"go.uber.org/zap"
)

type ProxyDB struct {
	log  *zap.Logger
	sink chan<- domain.ProvidedProxy
}

func NewProxyDB() *ProxyDB {
	return &ProxyDB{}
}

func (p *ProxyDB) Init(log *zap.Logger, sink chan<- domain.ProvidedProxy) {
	p.log = log
	p.sink = sink
}

type Proxy struct {
	IP             string  `json:"ip"`
	Port           int     `json:"port"`
	Type           string  `json:"type"`
	City           *string `json:"city"`
	CountryCode    string  `json:"ccode"`
	ISP            string  `json:"isp"`
	ResponseTime   float64 `json:"response_time"`
	Uptime         float64 `json:"uptime"`
	AnonymityLevel int     `json:"anonlvl"`
}

type proxyResponse struct {
	Proxies    []Proxy `json:"proxies"`
	TotalCount int     `json:"total_count"`
}

func (p *ProxyDB) Reconcile(ctx context.Context) (int, error) {
	return 0, p.LoadProxies(ctx, []string{}, []int{})
}

// LoadProxies pages through the API, retrying on 429, and uses the
// handler function stored in the ProxyDB struct.
func (p *ProxyDB) LoadProxies(
	ctx context.Context,
	protocols []string,
	anonLvls []int,
) error {
	offset := 0
	apiURL := "https://proxydb.net/list"

	for {
		bodyBytes, err := json.Marshal(map[string]interface{}{
			"protocols": protocols,
			"anonlvls":  anonLvls,
			"offset":    offset,
		})
		if err != nil {
			return fmt.Errorf("marshal body: %w", err)
		}

		req, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewReader(bodyBytes))
		if err != nil {
			return fmt.Errorf("new request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := utils.DoWithRetry(ctx, http.DefaultClient, req, backoff.NewExponentialBackOff())
		if err != nil {
			return fmt.Errorf("request failed: %w", err)
		}
		defer resp.Body.Close()

		var pr proxyResponse
		if err := json.NewDecoder(resp.Body).Decode(&pr); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}

		for _, proxy := range pr.Proxies {
			if err := p.handleFn(ctx, proxy); err != nil {
				return err
			}
		}

		offset += len(pr.Proxies)
		if offset >= pr.TotalCount || len(pr.Proxies) == 0 {
			break
		}

		select {
		case <-time.After(100 * time.Millisecond):
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	return nil
}

func (p *ProxyDB) handleFn(ctx context.Context, proxy Proxy) error {
	cleanedIP, port, err := utils.CleanHostPort(proxy.IP + ":" + strconv.Itoa(proxy.Port))
	if err != nil {
		p.log.Warn("Invalid IP address after cleaning", zap.String("host", "proxydb"), zap.Error(err))
		return nil
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case p.sink <- domain.ProvidedProxy{
		IP:       cleanedIP,
		Port:     port,
		Provider: "proxydb",
	}:
	}

	return nil
}
