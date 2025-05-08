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

type HostPortList struct {
	source string
	log    *zap.Logger
	sink   chan<- domain.ProvidedProxy
}

func NewHostPortList(source string) domain.ProxyProvider {
	return &HostPortList{
		source: source,
	}
}

func (ps *HostPortList) Init(log *zap.Logger, sink chan<- domain.ProvidedProxy) {
	ps.log = log
	ps.sink = sink
}

func (ps *HostPortList) Reconcile(ctx context.Context) error {
	parsedURL, err := url.Parse(ps.source)
	if err != nil {
		ps.log.Error("Invalid URL", zap.String("source", ps.source), zap.Error(err))
		return err
	}

	ps.log.Debug("Starting reconciliation", zap.String("host", parsedURL.Host))

	req, err := http.NewRequestWithContext(ctx, "GET", ps.source, nil)
	if err != nil {
		ps.log.Error("Request creation failed", zap.Error(err))
		return err
	}

	resp, err := utils.DoWithRetry(ctx, http.DefaultClient, req, backoff.NewExponentialBackOff())
	if err != nil {
		ps.log.Error("Fetch failed", zap.Error(err))
		return err
	}
	defer resp.Body.Close()

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		cleanedIP, portInt, err := utils.CleanHostPort(line)
		if err != nil {
			ps.log.Warn("Invalid IP", zap.String("host", line), zap.Error(err))
			continue
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case ps.sink <- domain.ProvidedProxy{
			IP:       cleanedIP,
			Port:     portInt,
			Provider: parsedURL.Host,
		}:
		}
	}

	if err := scanner.Err(); err != nil {
		ps.log.Warn("Scan error", zap.Error(err))
		return err
	}

	ps.log.Debug("Reconciliation complete", zap.String("host", parsedURL.Host))
	return nil
}
