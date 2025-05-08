package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/yuridevx/poe2scrap/pkg/models"
	"net/http"
	"strconv"
	"time"

	"github.com/cenkalti/backoff/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/yuridevx/poe2scrap/pkg/utils"
	"go.uber.org/zap"
)

type ProxyDB struct {
	db  *pgxpool.Pool
	log *zap.Logger
}

func NewProxyDB(
	log *zap.Logger,
	pool *pgxpool.Pool,
) *ProxyDB {
	return &ProxyDB{
		db:  pool,
		log: log,
	}
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

	switch proxy.Type {
	case "http", "https":
	case "socks4", "socks5":
	default:
		p.log.Warn("Unsupported scheme", zap.String("host", "proxydb"), zap.String("scheme", proxy.Type))
		return nil
	}

	q := models.New(p.db)

	err = q.NotifyProxyCandidate(ctx, models.NotifyProxyCandidateParams{
		Ip:       cleanedIP,
		Port:     int32(port),
		Protocol: proxy.Type,
		ProcessedAt: pgtype.Timestamp{
			Time:  time.Now(),
			Valid: true,
		},
		AllowRetryAt: pgtype.Timestamp{
			Time:  time.Now().Add(time.Hour * 24),
			Valid: true,
		},
	})
	if err != nil {
		p.log.Error("Insert failed", zap.String("host", "proxydb"), zap.String("proxy", cleanedIP+":"+strconv.Itoa(proxy.Port)), zap.Error(err))
		return err
	}
	return nil
}
