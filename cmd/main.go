package main

import (
	"context"
	"github.com/cenkalti/backoff/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/yuridevx/poe2scrap/pkg/proxysource"
	"github.com/yuridevx/poe2scrap/pkg/proxysource/providers"
	"github.com/yuridevx/poe2scrap/pkg/reconciler"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func initializeLogger(conf *config.Config) (*zap.Logger, error) {
	var logConfig zap.Config
	if conf.ZapProduction {
		logConfig = zap.NewProductionConfig()
	} else {
		logConfig = zap.NewDevelopmentConfig()
	}

	switch conf.ZapLogLevel {
	case "debug":
		logConfig.Level = zap.NewAtomicLevelAt(zapcore.DebugLevel)
	case "info":
		logConfig.Level = zap.NewAtomicLevelAt(zapcore.InfoLevel)
	case "warn":
		logConfig.Level = zap.NewAtomicLevelAt(zapcore.WarnLevel)
	case "error":
		logConfig.Level = zap.NewAtomicLevelAt(zapcore.ErrorLevel)
	default:
		logConfig.Level = zap.NewAtomicLevelAt(zapcore.InfoLevel) // Default to info
	}

	return logConfig.Build()
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	conf := config.NewConfig()

	// Initialize logger using the new function
	logger, err := initializeLogger(conf)
	if err != nil {
		panic(err)
	}

	db, err := pgxpool.New(ctx, conf.DSN)
	if err != nil {
		panic(err)
	}

	proxyDb := providers.NewProxyDB(logger, db)
	urlLists := providers.NewKnowUrlLists(logger, db)
	hostLists := providers.NewKnownHostPortList(logger, db)

	for _, urlList := range urlLists {
		reconciler.RunReconciler(
			ctx,
			urlList.Reconcile,
			reconciler.WithFailBackOff(&backoff.ConstantBackOff{Interval: time.Hour}),
			reconciler.WithWaitBackOff(&backoff.ConstantBackOff{Interval: time.Hour}),
		)
	}

	for _, hostList := range hostLists {
		reconciler.RunReconciler(
			ctx,
			hostList.Reconcile,
			reconciler.WithFailBackOff(&backoff.ConstantBackOff{Interval: time.Hour}),
			reconciler.WithWaitBackOff(&backoff.ConstantBackOff{Interval: time.Hour}),
		)
	}

	reconciler.RunReconciler(
		ctx,
		proxyDb.Reconcile,
		reconciler.WithFailBackOff(&backoff.ConstantBackOff{Interval: time.Hour}),
		reconciler.WithWaitBackOff(&backoff.ConstantBackOff{Interval: time.Hour}),
	)

	proxyCandidateReconciler := proxysource.NewProxyReconciler(logger, db, proxysource.NewProxyTest(logger, db))
	reconciler.RunReconciler(
		ctx,
		proxyCandidateReconciler.Reconcile,
		reconciler.WithFailBackOff(&backoff.ConstantBackOff{Interval: time.Second}),
		reconciler.WithWaitBackOff(&backoff.ConstantBackOff{Interval: time.Minute}),
	)

	<-ctx.Done()
}
