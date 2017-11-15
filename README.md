# TimescaleDB vs PostgreSQL Benchmark

## Introduction
This repository contains a collection of Go programs that can be used to
benchmark [TimescaleDB][timescaledb] against PostgreSQL on insert,
query, and deletion (data retention) performance. Additionally, we
provide a [data set and queries][dataset] to allow you to measure
performance [on the same data we have measured][blog].

## Getting Started
 You will need the Go runtime (1.6+) installed on the machine
 you wish to benchmark. You can access this repo via `go get`:
 ```bash
 go get github.com/timescale/benchmark-postgres
 ```

There are three programs available for installation under the `cmd`
directory, each of which can be installed with `go install`:
```bash
# Change to program directory
cd $GOPATH/src/github.com/timescale/benchmark-postgres/cmd/timescaledb-benchmark-query
go get .
go install

# Repeat for other programs
```

## Our Dataset

In a [discussion of how TimescaleDB compares to PostgreSQL][blog], we
used two datasets: one with 100M rows of CPU metrics and one with 1B
rows. We have made the [100M row dataset available][dataset]
(link will download 7GB archive) and use
it throughout this README as an example.
In addition to the data in CSV format, the archive
also contains a file to create the table schema we used and a selection
of queries we tested.

The rows represent CPU metrics for 4000 hosts over the course of 3 days,
`2016-01-01` through `2016-01-03`. Each row consists of a timestamp, a
host identifier, and 10 CPU metrics. All hosts have a row every 10
seconds for the duration of the 3 days, leading to just over 100M rows
of data. The CSV is **20 GB** and when imported the database
is **~30GB**.

To unpack the archive:
```bash
tar -vxjf benchmark_postgres.tar.bz2
```

This will unpack the following files into your current directory:

* `cpu-data.csv`
* `benchmark-setup-timescaledb.sql`
* `benchmark-setup-postgresql.sql`
* `queries-1-host-12-hr.sql`
* `queries-8-host-1-hr.sql`
* `queries-groupby-orderby-limit.sql`
* `queries-groupby.sql`

## Usage

### Benchmark: Inserts (timescaledb-parallel-copy)
In the `cmd` folder is a Git submodule to our [parallel copy][] program
that is generally available. This program can actually double as a way
to benchmark insert performance in either TimescaleDB or PostgreSQL.
Make sure it is installed (see above) and you are ready to go.

Using our 100M dataset, first you need to setup the database and tables.
Create a database in PostgreSQL called, e.g., `benchmark`. Then, setup
the tables using our provided schema:
```bash
# To setup a TimescaleDB hypertable
psql -d benchmark < benchmark-setup-timescaledb.sql
# To setup a plain PostgreSQL table
psql -d benchmark < benchmark-setup-postgresql.sql
```

Note that you can setup both tables in the same database for easy
comparisons. To measure insert performance, run
`timescaledb-parallel-copy` with the `--verbose` flag, and optionally
a `--reporting-period` to get in-progress results:
```bash
# For TimescaleDB, report every 30s
timescaledb-parallel-copy --db-name=benchmark --table=cpu_ts \
    --verbose --reporting-period=30s --file=cpu-data.csv

# For PostgreSQL, report every 30s
timescaledb-parallel-copy --db-name=benchmark --table=cpu_pg \
    --verbose --reporting-period=30s --file=cpu-data.csv
```

Once the copy is finished, you'll be given an average number of rows
per second over the whole insertion process. If you included a
`--reporting-period` you can also see how the performance changes over
time.

```bash
at 20s, row rate 137950.621224/sec (period), row rate 137950.621224/sec (overall), 2.760000E+06 total rows
at 40s, row rate 106634.481501/sec (period), row rate 122305.233000/sec (overall), 4.890000E+06 total rows
...
at 14m40s, row rate 97544.843755/sec (period), row rate 112289.740923/sec (overall), 9.877000E+07 total rows
at 15m0s, row rate 134560.614457/sec (period), row rate 112784.651475/sec (overall), 1.014600E+08 total rows
COPY 103680000, took 15m18.895179s with 8 worker(s) (mean rate 112831.150209/sec)
```

### Benchmark: Queries (timescaledb-benchmark-query)

To benchmark query latency, we provide `timescaledb-benchmark-query`,
which takes a file of queries (one per each line) and runs them in
parallel. Each query is run twice, to generate 'cold' and 'warm'
measurements for each, and the averages of both are printed after
all the queries are run:
```bash
timescaledb-benchmark-query --db-name=benchmark --table=cpu_ts \
    --query-file=queries-1-host-12-hr.sql --workers=1

avg of 'cold' 10 queries (ms):    19.90
avg of 'warm' 10 queries (ms):    14.50
```

You can compare these numbers to PostgreSQL by changing the `--table`
flag to point to a plain PostgreSQL table.

Included in our 100M row dataset are queries of 4 types:

* `1-host-12-hr`: Returns the max CPU usage per minute for 12 hours on one host
* `8-host-1-hr`: Same as above except for 8 eights over 1 hour
* `groupby`: Returns the avg CPU usage per host per hour over 24 hours
* `groupby-orderby-limit`: Returns the last 5 max CPU usage per minute across all devices with a random time range end point

_Note: The queries are missing the table name (replaced with %s), which
is filled in later by the `--table` flag to `timescaledb-benchmark-query`._

### Benchmark: Data Retention (timescaledb-benchmark-delete)

Our final benchmark deals with measuring the cost of removing data after
it falls outside of a retention period. TimescaleDB introduces a
function called `drop_chunks()` to easily remove data older than a
certain date. Combined with the way TimescaleDB organizes and stores
data, this is much more efficient for removing data.

To measure this, we provide `timescaledb-benchmark-delete` which can be
used to delete data using `drop_chunks()` or SQL's `DELETE` command.
Note that this program **does actually delete the data**, so if you
using it, make sure the data loss is okay (i.e. **DO NOT USE** on
production data).

The program requires a start date from which to delete data, an amount
to delete -- which should be equal to a chunk size for TimescaleDB,
and the number of times to delete that amount of data. Our 100M row
dataset begins on `2016-01-01` at midnight, so that is the start date
we'll use along with 12-hour chunks. So, to delete the first 3 chunks
we run:
```bash
timescaledb-benchmark-delete --db-name=benchmark --table=cpu_ts \
    --start="2016-01-01T00:00:00Z" --amount="12h" --limit=3
```
This will print out the command used and time it took to execute in
milliseconds:
```bash
SELECT drop_chunks('2016-01-01T12:00:00Z'::TIMESTAMPTZ, 'cpu_ts')
66ms

SELECT drop_chunks('2016-01-02T00:00:00Z'::TIMESTAMPTZ, 'cpu_ts')
19ms

SELECT drop_chunks('2016-01-02T12:00:00Z'::TIMESTAMPTZ, 'cpu_ts')
19ms
```
To benchmark the equivalent scenario on PostgreSQL requires you to
disable the use of `drop_chunks()`:
```bash
timescaledb-benchmark-delete --db-name=benchmark --table=cpu_pg \
    --start="2016-01-01T00:00:00Z" --amount="12h" --limit=3 \
    --use-drop-chunks=false
```

[timescaledb]: https://github.com/timescale/timescaledb
[dataset]: https://timescaledata.blob.core.windows.net/datasets/benchmark_postgres.tar.bz2
[blog]: https://blog.timescale.com/timescaledb-vs-6a696248104e
[parallel copy]: https://github.com/timescale/timescaledb-parallel-copy
