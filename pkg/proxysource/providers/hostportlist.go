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
	"strconv"
	"time"
)

type HostPortList struct {
	scheme string
	source string
	db     *pgxpool.Pool
	log    *zap.Logger
}

func NewHostPortList(schema string, source string, log *zap.Logger, db *pgxpool.Pool) *HostPortList {
	return &HostPortList{
		scheme: schema,
		source: source,
		log:    log,
		db:     db,
	}
}

func (ps *HostPortList) Reconcile(ctx context.Context) (int, error) {
	parsedURL, err := url.Parse(ps.source)
	if err != nil {
		ps.log.Error("Invalid URL", zap.String("source", ps.source), zap.Error(err))
		return 0, err
	}

	ps.log.Debug("Starting reconciliation", zap.String("host", parsedURL.Host))

	req, err := http.NewRequestWithContext(ctx, "GET", ps.source, nil)
	if err != nil {
		ps.log.Error("Request creation failed", zap.Error(err))
		return 0, err
	}

	resp, err := utils.DoWithRetry(ctx, http.DefaultClient, req, backoff.NewExponentialBackOff())
	if err != nil {
		ps.log.Error("Fetch failed", zap.Error(err))
		return 0, err
	}
	defer resp.Body.Close()

	q := models.New(ps.db)

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		cleanedIP, portInt, err := utils.CleanHostPort(line)
		if err != nil {
			ps.log.Warn("Invalid IP", zap.String("host", line), zap.Error(err))
			continue
		}

		err = q.NotifyProxyCandidate(ctx, models.NotifyProxyCandidateParams{
			Ip:       cleanedIP,
			Port:     int32(portInt),
			Protocol: ps.scheme,
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
			ps.log.Error("Insert failed", zap.String("proxy", cleanedIP+":"+strconv.Itoa(portInt)), zap.Error(err))
			return 0, err
		}
		ps.log.Debug("Proxy inserted", zap.String("proxy", cleanedIP+":"+strconv.Itoa(portInt)))
	}

	if err := scanner.Err(); err != nil {
		ps.log.Warn("Scan error", zap.Error(err))
		return 0, err
	}

	ps.log.Debug("Reconciliation complete", zap.String("host", parsedURL.Host))
	return 0, nil
}
