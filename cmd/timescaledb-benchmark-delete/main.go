// timescaledb-benchmark-delete measures the time (in milliseconds) to delete data from two systems:
// (1) TimescaleDB (by using the default --use-drop-chunks flag)
// (2) PostgreSQL (by setting the --use-drop-chunks flag to 'false')
//
// Typical use for testing TimescaleDB
// timescaledb-benchmark-delete --amount=6h --limit=5
//
// This will drop five 6-hour intervals from default database & table starting
// at the default start date, outputting the SQL command used and the time
// in milliseconds it took. By default, this uses TimescaleDB's `drop_chunks()`
// function.
//
// ----------
// Typical use for testing PostgreSQL
// timescaledb-benchmark-delete --amount=6h --limit=5 --use-drop-chunks=false
//
// This will drop five 6-hour intervals from default database & table starting
// at the default start date, outputting the SQL command used and the time
// in milliseconds it took. This will use a normal SQL `DELETE` command
//
package main

import (
	"flag"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
)

// Program flag vars
var (
	postgresConnect string
	databaseName    string
	table           string

	dropChunks bool
	startDate  string
	amount     time.Duration
	limit      uint
)

const goTimeFmt = "2006-01-02T15:04:05Z"

// Parse flags
func init() {
	flag.StringVar(&postgresConnect, "postgres", "host=localhost user=postgres sslmode=disable", "Postgres connection url")
	flag.StringVar(&databaseName, "db-name", "benchmark", "Name of database to connect to")
	flag.StringVar(&table, "table", "test", "Table to remove data from")

	flag.BoolVar(&dropChunks, "use-drop-chunks", true, "Whether to use TimescaleDB function drop_chunks(). Set to false to test PostgreSQL")
	flag.StringVar(&startDate, "start", "2016-01-01T00:00:00Z", "Start date to delete from using the form YYYY-MM-DDThh:mm:ssZ")
	flag.DurationVar(&amount, "amount", 1*time.Hour, "Amount of data to delete given as a duration of time (e.g. 3h = 3 hours worth)")
	flag.UintVar(&limit, "limit", 1, "Number of times to delete --amount of data starting from --start")

	flag.Parse()
}

func getConnectString() string {
	return postgresConnect + " dbname=" + databaseName
}

func getDeleteQuery(t time.Time) string {
	if dropChunks {
		return fmt.Sprintf("SELECT drop_chunks('%s'::TIMESTAMPTZ, '%s')", t.Add(amount).Format(goTimeFmt), table)
	}
	end := t.Add(amount)
	return fmt.Sprintf("DELETE FROM \"%s\" WHERE time >= '%v' AND time < '%v'", table, t.Format(goTimeFmt), end.Format(goTimeFmt))
}

func main() {
	db := sqlx.MustConnect("postgres", getConnectString())
	defer db.Close()

	// Parse starting date for delete queries
	rangeStart, err := time.Parse(goTimeFmt, startDate)
	if err != nil {
		panic(fmt.Sprintf("could not parse start date: %v\n", err))
	}

	// Delete data sized by --amount (in terms of time) and for --limit iterations
	for i := uint(0); i < limit; i++ {
		start := time.Now()
		q := getDeleteQuery(rangeStart)
		rows, err := db.Queryx(q)
		if err != nil {
			panic(err)
		}
		rows.Close()

		// Calculate and print elapsed time
		took := time.Now().Sub(start)
		fmt.Printf("%s\n%vms\n\n", q, took.Nanoseconds()/1e6)

		// Update starting time for potential next iteration
		rangeStart = rangeStart.Add(amount)
	}
}
