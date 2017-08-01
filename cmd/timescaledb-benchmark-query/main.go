// timescaledb-benchmark-query measures the time (in milliseconds) to run SQL queries on a PostgreSQL
// based-system, for example:
// (1) TimescaleDB
// (2) PostgreSQL
//
// As input, it takes a file with a query on each line to run, and runs them twice
// back-to-back to get 'cold' and 'warm' latency numbers. These queries should be
// of similar construction for the most useful numbers. That is, given a query type
// it should need roughly the same number of rows to answer. For example, asking for
// the average of a metric per minute over an hour and varying which hour to query
// is a better query construction than asking for the average of a metric per minute
// over a variable time period.
//
// Typical use for testing
// timescaledb-benchmark-delete --query-file=foo.sql
//
// This will run all the queries in foo.sql on the default database and table.
// You can change the database with --db-name or the table with --table to try
// it on non-TimescaleDB databases/tables vs TimescaleDB databases/tables.
//
package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
)

// Program flag vars
var (
	postgresConnect string
	databaseName    string
	table           string

	queryFile string
	workers   uint64
)

var (
	queryChan    chan string
	workersGroup sync.WaitGroup

	cold int64
	warm int64
	cnt  int64
)

// Parse flags
func init() {
	flag.StringVar(&postgresConnect, "postgres", "host=localhost user=postgres sslmode=disable", "Postgres connection url")
	flag.StringVar(&databaseName, "db-name", "benchmark", "Name of database to connect to")
	flag.StringVar(&table, "table", "test", "Table to remove data from")

	flag.StringVar(&queryFile, "query-file", "", "Path to file containing queries to execute")
	flag.Uint64Var(&workers, "workers", 1, "Number of parallel queries to run")
	flag.Parse()
}

func getConnectString() string {
	return postgresConnect + " dbname=" + databaseName
}

func main() {
	if len(queryFile) <= 0 {
		fmt.Println("ERROR: --query-file not specified")
		os.Exit(1)
	}
	var scanner *bufio.Scanner
	file, err := os.Open(queryFile)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()
	queryChan = make(chan string, workers)

	scanner = bufio.NewScanner(file)
	rows := make([]string, 0)
	for scanner.Scan() {
		rows = append(rows, scanner.Text())
	}

	if err := scanner.Err(); err != nil {
		log.Fatalf("Error reading input: %s", err.Error())
	}

	for i := uint64(0); i < workers; i++ {
		workersGroup.Add(1)
		go processQueries()
	}

	for _, r := range rows {
		queryChan <- r
	}
	close(queryChan)
	workersGroup.Wait()

	fmt.Printf("avg of 'cold' %d queries (ms): %8.2f\n", cnt, float64(cold)/float64(cnt))
	fmt.Printf("avg of 'warm' %d queries (ms): %8.2f\n", cnt, float64(warm)/float64(cnt))
}

func processQueries() {
	db := sqlx.MustConnect("postgres", getConnectString())
	defer db.Close()

	sumCold := int64(0)
	sumWarm := int64(0)
	count := int64(0)
	for qFmt := range queryChan {
		q := fmt.Sprintf(qFmt, table)
		start := time.Now()
		res, err := db.Queryx(q)
		if err != nil {
			panic(err)
		}
		res.Close()
		took := time.Now().Sub(start)
		sumCold += took.Nanoseconds() / 1e6

		start = time.Now()
		res, err = db.Queryx(q)
		if err != nil {
			panic(err)
		}
		res.Close()
		took = time.Now().Sub(start)
		sumWarm += took.Nanoseconds() / 1e6

		count++
	}

	atomic.AddInt64(&cold, sumCold)
	atomic.AddInt64(&warm, sumWarm)
	atomic.AddInt64(&cnt, count)
	workersGroup.Done()
}
