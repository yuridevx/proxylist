package proxysource

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/yuridevx/poe2scrap/pkg/models"
	"go.uber.org/zap"
	"golang.org/x/sync/semaphore"
)

const (
	pageSize       = 50
	maxConcurrency = 50
	testTimeout    = 30 * time.Second
)

type ProxyReconciler struct {
	log  *zap.Logger
	db   *pgxpool.Pool
	test *ProxyTest
}

func NewProxyReconciler(log *zap.Logger, db *pgxpool.Pool, test *ProxyTest) *ProxyReconciler {
	return &ProxyReconciler{log: log, db: db, test: test}
}

func (pr *ProxyReconciler) Reconcile(ctx context.Context) (int, error) {
	q := models.New(pr.db)

	total, err := q.CountProxyCandidateForTest(ctx)
	if err != nil {
		return 0, fmt.Errorf("count proxies: %w", err)
	}

	var (
		wg  sync.WaitGroup
		sem = semaphore.NewWeighted(maxConcurrency)
	)

	for offset := 0; offset < int(total); offset += pageSize {
		candidates, err := q.ListProxyCandidateForTest(ctx, models.ListProxyCandidateForTestParams{
			Limit:  int32(pageSize),
			Offset: int32(offset),
		})
		if err != nil {
			return 0, fmt.Errorf("list proxies @ offset %d: %w", offset, err)
		}
		if len(candidates) == 0 {
			break
		}

		for _, cand := range candidates {
			// stop enqueueing if parent context is done
			if err := sem.Acquire(ctx, 1); err != nil {
				break
			}

			wg.Add(1)
			go func(c models.ListProxyCandidateForTestRow) {
				defer wg.Done()
				defer sem.Release(1)

				testCtx, cancel := context.WithTimeout(ctx, testTimeout)
				defer cancel()

				if err := pr.test.Test(testCtx, c.Ip, c.Port, c.Protocol); err != nil {
				}
			}(cand)
		}
	}

	wg.Wait()
	pr.log.Info("finished proxy reconciliation")
	return int(total), nil
}
