package providers

import (
	"bufio"
	"context"
	"github.com/cenkalti/backoff/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/yuridevx/poe2scrap/pkg/models"
	"github.com/yuridevx/poe2scrap/pkg/utils"
	"go.uber.org/zap"
	"net/http"
	"net/url"
	"time"
)

type UrlList struct {
	source string
	db     *pgxpool.Pool
	log    *zap.Logger
}

func NewUrlProxyList(source string, log *zap.Logger, db *pgxpool.Pool) *UrlList {
	return &UrlList{
		source: source,
		log:    log,
		db:     db,
	}
}

func (ps *UrlList) Reconcile(ctx context.Context) (int, error) {
	parsedUrl, err := url.Parse(ps.source)
	if err != nil {
		ps.log.Error("Invalid source URL", zap.String("source", ps.source), zap.Error(err))
		return 0, err
	}
	host := parsedUrl.Host

	ps.log.Debug("Starting reconciliation", zap.String("host", host))

	req, err := http.NewRequest("GET", ps.source, nil)
	if err != nil {
		ps.log.Error("Request creation failed", zap.String("host", host), zap.Error(err))
		return 0, err
	}

	resp, err := utils.DoWithRetry(ctx, http.DefaultClient, req, backoff.NewExponentialBackOff())
	if err != nil {
		ps.log.Error("Fetch failed", zap.String("host", host), zap.Error(err))
		return 0, err
	}
	defer resp.Body.Close()

	q := models.New(ps.db)

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		ps.log.Debug("Processing line", zap.String("host", host))

		proxyUrl, err := url.Parse(line)
		if err != nil {
			ps.log.Warn("Parse failed", zap.String("host", host), zap.Error(err))
			continue
		}

		cleanedIP, portInt, err := utils.CleanHostPort(proxyUrl.Host)
		if err != nil {
			ps.log.Warn("Invalid IP address after cleaning", zap.String("host", host), zap.Error(err))
			continue
		}

		switch proxyUrl.Scheme {
		case "http", "https":
		case "socks4", "socks5":
		default:
			ps.log.Warn("Unsupported scheme", zap.String("host", host), zap.String("scheme", proxyUrl.Scheme))
			continue
		}

		err = q.NotifyProxyCandidate(ctx, models.NotifyProxyCandidateParams{
			Ip:       cleanedIP,
			Port:     int32(portInt),
			Protocol: proxyUrl.Scheme,
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
			ps.log.Error("Insert failed", zap.String("host", host), zap.String("proxy", cleanedIP+":"+proxyUrl.Port()), zap.Error(err))
			return 0, err
		}
		ps.log.Debug("Inserted candidate", zap.String("host", host), zap.String("proxy", cleanedIP+":"+proxyUrl.Port()))
	}

	if err := scanner.Err(); err != nil {
		ps.log.Warn("Scan error", zap.String("host", host), zap.Error(err))
		return 0, err
	}

	ps.log.Debug("Reconciliation complete", zap.String("host", host))
	return 0, nil //0 to trigger wait
}
