package providers

import (
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

func NewKnowUrlLists(log *zap.Logger, db *pgxpool.Pool) []*UrlList {
	return []*UrlList{
		NewUrlProxyList("https://api.proxyscrape.com/v4/free-proxy-list/get"+
			"?request=display_proxies&proxy_format=protocolipport&format=text",
			log, db),
		NewUrlProxyList("https://raw.githubusercontent.com/proxifly/free-proxy-list/"+
			"refs/heads/main/proxies/all/data.txt",
			log, db),
	}
}

func NewKnownHostPortList(log *zap.Logger, db *pgxpool.Pool) []*HostPortList {
	return []*HostPortList{
		NewHostPortList(
			"http", "https://www.proxy-list.download/api/v1/get?type=http",
			log, db,
		),
		NewHostPortList(
			"https", "https://www.proxy-list.download/api/v1/get?type=https",
			log, db,
		),
		NewHostPortList(
			"socks4", "https://www.proxy-list.download/api/v1/get?type=socks4",
			log, db,
		),
		NewHostPortList(
			"socks5", "https://www.proxy-list.download/api/v1/get?type=socks5",
			log, db,
		),
		NewHostPortList(
			"http", "https://vakhov.github.io/fresh-proxy-list/http.txt",
			log, db,
		),
		NewHostPortList(
			"https", "https://vakhov.github.io/fresh-proxy-list/https.txt",
			log, db,
		),
		NewHostPortList(
			"socks4", "https://vakhov.github.io/fresh-proxy-list/socks4.txt",
			log, db,
		),
		NewHostPortList(
			"socks5", "https://vakhov.github.io/fresh-proxy-list/socks5.txt",
			log, db,
		),
	}
}
