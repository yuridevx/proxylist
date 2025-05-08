-- drop table proxy_info;

CREATE TABLE proxy_info
(
    ip                    varchar(39),
    port                  int,
    protocol              varchar(10),
    delay interval,
    tested_at             timestamp,
    websocket             boolean,
    anonymity             bool,
    item_fetch            bool,

    fetch_error_count     int,
    websocket_error_count int,

    primary key (ip, port, protocol)
);

--drop table proxy_candidate;

CREATE TABLE proxy_candidate
(
    ip             varchar(39),
    port           integer,
    processed_at   timestamp,
    tested_at      timestamp null default null,
    allow_retry_at timestamp,
    test_error     varchar default '',
    primary key (ip, port)
);