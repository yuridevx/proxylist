-- name: InsertProxyInfoTestResults :exec
insert into proxy_info (ip, port, protocol, provider, delay_ms, tested_at, websocket, anonymity, item_fetch)
values ($1, $2, $3, $4, $5, $6, $7, $8, $9)
on conflict (ip, port, protocol) do update
    set delay_ms   = EXCLUDED.delay_ms,
        tested_at  = EXCLUDED.tested_at,
        websocket  = EXCLUDED.websocket,
        anonymity  = EXCLUDED.anonymity,
        item_fetch = EXCLUDED.item_fetch;

-- name: ProxyInfoWebsocketDisconnect :exec
update proxy_info
set websocket_error_count = websocket_error_count + 1
where ip = $1
  and port = $2
  and protocol = $3;

-- name: ProxyInfoFetchError :exec
update proxy_info
set fetch_error_count = fetch_error_count + 1
where ip = $1
  and port = $2
  and protocol = $3;


