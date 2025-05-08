package domain

import (
	"context"
	"go.uber.org/zap"
	"strconv"
)

type ProvidedProxy struct {
	IP       string
	Port     int
	Provider string
}

func (p ProvidedProxy) String() string {
	return p.IP + ":" + strconv.Itoa(p.Port) + " (" + p.Provider + ")"
}

type ProxyProvider interface {
	Init(log *zap.Logger, sink chan<- ProvidedProxy)
	Reconcile(ctx context.Context) error
}
