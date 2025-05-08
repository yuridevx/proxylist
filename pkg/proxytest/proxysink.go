package proxytest

import (
	"context"
	"github.com/yuridevx/proxylist/pkg/dedup"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/yuridevx/proxylist/domain"
	"github.com/yuridevx/proxylist/pkg/models"
	"go.uber.org/zap"
)

// ProxySink reads proxies from the 'in' channel and
// dispatches them to a fixed pool of workers.
type ProxySink struct {
	in       <-chan domain.ProvidedProxy
	fetchUrl string
	log      *zap.Logger
	db       *pgxpool.Pool
	workers  int
	wg       sync.WaitGroup
	timeoutS int
	de       *dedup.Deduplicator
}

// NewProxySink wires up a sink with 'n' concurrent workers.
func NewProxySink(in <-chan domain.ProvidedProxy, log *zap.Logger, db *pgxpool.Pool, de *dedup.Deduplicator, fetchUrl string, n int, timeoutS int) *ProxySink {
	return &ProxySink{
		in:       in,
		log:      log,
		db:       db,
		de:       de,
		fetchUrl: fetchUrl,
		timeoutS: timeoutS,
		workers:  n,
	}
}

// Start spins up the worker goroutines. Call Stop() after closing 'in'.
func (s *ProxySink) Start(ctx context.Context) {
	for i := 0; i < s.workers; i++ {
		s.wg.Add(1)
		go s.worker(ctx, i)
	}
}

// Stop blocks until all workers have exited.
func (s *ProxySink) Stop() {
	s.wg.Wait()
}

// worker pulls proxies off the channel and processes them.
func (s *ProxySink) worker(ctx context.Context, id int) {
	defer s.wg.Done()
	checker := NewProxyChecker(s.fetchUrl, s.timeoutS)
	repo := models.New(s.db)

	for {
		select {
		case <-ctx.Done():
			s.log.Info("worker shutting down", zap.Int("id", id))
			return
		case proxy, ok := <-s.in:
			if !ok {
				s.log.Info("input channel closed", zap.Int("worker", id))
				return
			}
			s.processOne(ctx, checker, repo, id, proxy)
		}
	}
}

// processOne does the Check + DB insert for a single proxy.
func (s *ProxySink) processOne(
	ctx context.Context,
	checker *ProxyChecker,
	repo *models.Queries,
	workerID int,
	proxy domain.ProvidedProxy,
) {
	if !s.de.ShouldProcess(proxy, time.Hour*8) {
		return
	}

	defer func() {
		_ = s.de.MarkProcessed(proxy)
	}()

	defer func() {
		s.log.Info("finished proxy", zap.String("proxy", proxy.String()))
	}()

	res, err := checker.Check(ctx, proxy)
	if err != nil {
		//s.log.Warn("check failed",
		//	zap.Int("worker", workerID),
		//	zap.String("proxy", proxy.String()),
		//	zap.Error(err),
		//)
		return
	}

	if !res.Success {
		return
	}

	params := models.InsertProxyInfoTestResultsParams{
		Ip:       proxy.IP,
		Port:     int32(proxy.Port),
		Protocol: res.Proto.String(),
		Provider: pgtype.Text{
			String: proxy.Provider,
			Valid:  true,
		},
		DelayMs: pgtype.Int4{
			Int32: int32(res.Duration.Milliseconds()),
			Valid: true,
		},
		TestedAt: pgtype.Timestamp{
			Time:  time.Now(),
			Valid: true,
		},
		Websocket: pgtype.Bool{
			Bool:  res.WebSocket.Success,
			Valid: true,
		},
		Anonymity: pgtype.Bool{
			Bool:  !res.ExposesIP,
			Valid: true,
		},
		ItemFetch: pgtype.Bool{
			Bool:  res.FetchSuccess,
			Valid: true,
		},
	}

	err = repo.InsertProxyInfoTestResults(ctx, params)
	if err != nil {
		s.log.Error("failed to insert proxy info test results", zap.Any("proxy", proxy), zap.Error(err))
	}
}
