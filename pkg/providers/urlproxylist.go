package providers

import (
	"bufio"
	"context"
	"github.com/cenkalti/backoff/v5"
	"github.com/yuridevx/proxylist/domain"
	"github.com/yuridevx/proxylist/pkg/utils"
	"go.uber.org/zap"
	"net/http"
	"net/url"
	"strings"
)

type UrlList struct {
	source string
	log    *zap.Logger
	sink   chan<- domain.ProvidedProxy
}

func NewUrlProxyList(source string) *UrlList {
	return &UrlList{
		source: source,
	}
}

func (ps *UrlList) Init(log *zap.Logger, sink chan<- domain.ProvidedProxy) {
	ps.log = log
	ps.sink = sink
}

func (ps *UrlList) Reconcile(ctx context.Context) error {
	parsedUrl, err := url.Parse(ps.source)
	if err != nil {
		ps.log.Error("Invalid source URL", zap.String("source", ps.source), zap.Error(err))
		return err
	}
	host := parsedUrl.Host

	req, err := http.NewRequest("GET", ps.source, nil)
	if err != nil {
		ps.log.Error("Request creation failed", zap.String("host", host), zap.Error(err))
		return err
	}

	resp, err := utils.DoWithRetry(ctx, http.DefaultClient, req, backoff.NewExponentialBackOff())
	if err != nil {
		ps.log.Error("Fetch failed", zap.String("host", host), zap.Error(err))
		return err
	}
	defer resp.Body.Close()

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

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

		select {
		case <-ctx.Done():
			return ctx.Err()
		case ps.sink <- domain.ProvidedProxy{
			IP:       cleanedIP,
			Port:     portInt,
			Provider: host,
		}:
		}
	}

	if err := scanner.Err(); err != nil {
		ps.log.Warn("Scan error", zap.String("host", host), zap.Error(err))
		return err
	}

	ps.log.Debug("Reconciliation complete", zap.String("host", host))
	return nil
}
