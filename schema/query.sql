-- name: NotifyProxyTested :exec
INSERT INTO proxy_candidate (ip, port, protocol, processed_at, tested_at, allow_retry_at, test_error)
VALUES ($1, $2, $3, $4, $5, $6, $7) ON CONFLICT (ip, port, protocol) DO
UPDATE
    SET processed_at = EXCLUDED.processed_at,
    tested_at = excluded.tested_at,
    allow_retry_at = EXCLUDED.allow_retry_at,
    test_error = excluded.test_error;

-- name: NotifyProxyCandidate :exec
INSERT INTO proxy_candidate (ip, port, protocol, processed_at, allow_retry_at)
VALUES ($1, $2, $3, $4, $5) ON CONFLICT (ip, port, protocol) DO
UPDATE
    SET processed_at = EXCLUDED.processed_at,
    allow_retry_at = EXCLUDED.allow_retry_at
WHERE $4 > proxy_candidate.allow_retry_at;

-- name: ListProxyCandidateForTest :many
select ip, port, protocol
from proxy_candidate
where tested_at is null
order by ip, port, protocol limit $1
offset $2;

-- name: CountProxyCandidateForTest :one
select count(*)
from proxy_candidate
where tested_at is null;

-- name: InsertProxyInfoTestResults :exec
insert into proxy_info (ip, port, protocol, delay, tested_at, websocket, anonymity, item_fetch)
values ($1, $2, $3, $4, $5, $6, $7, $8) on conflict (ip, port, protocol) do
update
    set delay = EXCLUDED.delay,
    tested_at = EXCLUDED.tested_at,
    websocket = EXCLUDED.websocket,
    anonymity = EXCLUDED.anonymity,
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


