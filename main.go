package main

import (
	"database/sql"
	"log"
	"os"
	"time"

	"github.com/hidori/go-test-pgssi/pattern"
	_ "github.com/lib/pq"
)

const connectionString = "host=localhost port=15432 user=user password=password dbname=testdb sslmode=disable"

func main() {
	log.Println("==== Starting DirtyRead test...")
	invoke(pattern.NewDirtyRead(connectionString).Do)
	log.Println("==== DirtyRead completed ====")

	log.Println("==== Starting PhantomRead test...")
	invoke(pattern.NewPhantomRead(connectionString).Do)
	log.Println("==== PhantomRead completed ====")

	log.Println("==== Starting WriteSkew test...")
	invoke(pattern.NewWriteSkew(connectionString).Do)
	log.Println("==== WriteSkew completed ====")

	time.Sleep(100 * time.Millisecond)

	os.Stdout.Sync()
	os.Stderr.Sync()
}

func invoke(fn func(db *sql.DB, done chan struct{})) {
	db, err := sql.Open("postgres", connectionString)
	if err != nil {
		log.Println("**** Failed to create setup DB connection:", err)
		return
	}
	defer db.Close()

	_, err = db.Exec("DELETE FROM test;")
	if err != nil {
		log.Println("**** Failed to delete all records:", err)
		return
	}

	_, err = db.Exec(`INSERT INTO test (name) VALUES ('Alice'), ('Bob');`)
	if err != nil {
		log.Println("**** Failed to seed records:", err)
		return
	}

	done := make(chan struct{}, 2)
	fn(db, done)
	<-done
	<-done
}
