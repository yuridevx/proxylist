package domain

import (
	"context"
	"fmt"
	"go.uber.org/zap"
	"strconv"
)

type ProvidedProxy struct {
	IP       string
	Port     int
	Provider string
}

// Key serializes a proxy to a unique string
func (p ProvidedProxy) Key() []byte {
	return []byte(fmt.Sprintf("%s:%d", p.IP, p.Port))
}

func (p ProvidedProxy) String() string {
	return p.IP + ":" + strconv.Itoa(p.Port) + " (" + p.Provider + ")"
}

type ProxyProvider interface {
	Init(log *zap.Logger, sink chan<- ProvidedProxy)
	Reconcile(ctx context.Context) error
}
