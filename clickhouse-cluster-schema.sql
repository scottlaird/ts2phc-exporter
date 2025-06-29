-- This creates a table for GPS satellite observations in a Clickhouse clustered database.
--
-- To use in a non-clustered environment:
--
-- 1.  Delete the definition of 'gps.satobservations' at the end.
-- 2.  Rename 'gps.satobservations_shard' to 'gps.satobservations' at the top.
-- 3.  Change the new gps.satobservations table to use the MergeTree engine instead of ReplicatedMergeTree
-- 4.  Remove all of the "on cluster custom" lines

create database gps on cluster custom;

CREATE or REPLACE TABLE gps.satobservations_shard on cluster custom
(
    timestamp DateTime64(9) CODEC(Delta(8), ZSTD(1)),
    constellation LowCardinality(String),
    name LowCardinality(String),
    band LowCardinality(String),
    frequency LowCardinality(String),
    satelliteid int,
    antenna LowCardinality(String),
    receiver LowCardinality(String),
    azimuth int,
    elev int,
    snr int,
)
ENGINE = ReplicatedMergeTree()
PARTITION BY toDate(timestamp)
ORDER BY (receiver, antenna, timestamp)
SETTINGS ttl_only_drop_parts = 1;

create or replace table gps.satobservations on cluster custom
as gps.satobservations_shard
ENGINE=Distributed(custom, gps, satobservations_shard, cityHash64(timestamp)) 
;


