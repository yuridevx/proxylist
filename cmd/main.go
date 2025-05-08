package main

import (
	"context"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/yuridevx/proxylist/domain"
	"github.com/yuridevx/proxylist/pkg/config"
	"github.com/yuridevx/proxylist/pkg/providers"
	"github.com/yuridevx/proxylist/pkg/proxytest"
	"github.com/yuridevx/proxylist/pkg/reconciler"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"os"
	"os/signal"
	"syscall"
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

	var sink = make(chan domain.ProvidedProxy)
	var proxyProviders []domain.ProxyProvider

	for _, urlSource := range conf.UrlSourceList {
		proxyProviders = append(proxyProviders, providers.NewUrlProxyList(urlSource))
	}
	for _, hostPortSource := range conf.HostPortSourceList {
		proxyProviders = append(proxyProviders, providers.NewHostPortList(hostPortSource))
	}

	for _, prov := range proxyProviders {
		prov.Init(logger, sink)
		reconciler.RunReconciler(ctx, prov.Reconcile)
	}

	proxySink := proxytest.NewProxySink(
		sink,
		logger,
		db,
		conf.FetchItemUrl,
		conf.ParallelTests,
		conf.ProxyTimeoutS,
	)
	proxySink.Start(ctx)

	<-ctx.Done()
}
