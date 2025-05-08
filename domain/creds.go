package domain

import "net/url"

type WebCredentials interface {
	ProxyRandomUrl() *url.URL
	UserAgent() string

	RotateSessionId(prevSessionId string) string
}
