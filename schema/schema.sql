-- drop table proxy_info;

CREATE TABLE proxy_info
(
    ip                    varchar(39),
    port                  int,
    protocol              varchar(10),
    provider              varchar,
    delay_ms               int,
    tested_at             timestamp,
    websocket             boolean,
    anonymity             bool,
    item_fetch            bool,

    fetch_error_count     int,
    websocket_error_count int,

    primary key (ip, port, protocol)
);